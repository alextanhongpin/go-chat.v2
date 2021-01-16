package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Socket struct {
	sync.RWMutex

	clients map[string]*Client
	issuer  ticket.Issuer
	rdb     *redis.Client
	friends *FriendService
}

func NewSocket(issuer ticket.Issuer) *Socket {
	s := &Socket{
		clients: make(map[string]*Client),
		issuer:  issuer,
		rdb:     NewRedis(),
		friends: NewFriendService(),
	}
	s.subscribe()
	return s
}

func (s *Socket) subscribe() {
	pubsub := s.rdb.Subscribe(context.Background(), channel)

	go func() {
		ch := pubsub.Channel()

		for msg := range ch {
			var m Message
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				log.Printf("unmarshalErr: %s\n", err)
				continue
			}
			to, ok := m.Payload["to"].(string)
			if !ok {
				continue
			}
			socketIDs, err := s.RoomMembers(to)
			if err != nil {
				log.Printf("RoomMembersErr: %s\n", err)
				continue
			}

			for _, socketID := range socketIDs {
				s.Publish(socketID, m)
			}
		}
	}()
}

func (s *Socket) authorize(r *http.Request) (string, error) {
	token := r.URL.Query().Get("token")
	user, err := s.issuer.Verify(token)
	if err != nil {
		return "", err
	}
	return user, nil
}

func (s *Socket) JoinRoom(socketID, room string) error {
	return s.rdb.SAdd(context.Background(), room, []string{socketID}).Err()
}

func (s *Socket) LeaveRoom(socketID, room string) error {
	return s.rdb.SRem(context.Background(), room, []string{socketID}).Err()
}

func (s *Socket) RoomMembers(room string) ([]string, error) {
	return s.rdb.SMembers(context.Background(), room).Result()
}

func (s *Socket) ServeWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgradeWebSocketErr: %s", err)
		return
	}

	// Authorize.
	user, err := s.authorize(r)
	if err != nil {
		ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("unauthorized: %s", err.Error())))
		return
	}

	client := NewClient()

	s.Register(client)
	defer s.Deregister(client)

	s.JoinRoom(client.ID, user)
	defer s.LeaveRoom(client.ID, user)

	// TODO: Notify presence.
	s.NotifyPresence(user, true)

	client.On("send_message", func(in map[string]interface{}) error {
		in["from"] = user
		in["to"] = user
		in["createdAt"] = time.Now()
		msg := Message{
			Type:    "message_sent",
			Payload: in,
		}
		b, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return s.rdb.Publish(context.Background(), channel, string(b)).Err()
		// Write to own socket.
		//client.Write(Message{
		//Type: "message_sent",
		//Payload: map[string]interface{}{
		//"msg":  in["msg"].(string),
		//"from": user,
		//"to":   user,
		//},
		//})
		// Publish to all connected clients.
		//s.Broadcast(msg)
		//
		// Publish to one specific client.
	})

	log.Println("connected:", client.ID)
	client.ServeWS(ws)
	log.Println("disconnected:", client.ID)
	s.NotifyPresence(user, false)

}

func (s *Socket) NotifyPresence(user string, online bool) {
	friends := s.friends.FindFriendsFor(user)

	for _, friend := range friends {
		msg := Message{
			Type: "presence_notified",
			Payload: map[string]interface{}{
				"to":       friend,
				"username": user,
				"online":   online,
			},
		}
		b, err := json.Marshal(msg)
		if err != nil {
			log.Printf("notifyPresenceMarhshalErr: %s\n", err)
			continue
		}

		if err := s.rdb.Publish(context.Background(), channel, string(b)).Err(); err != nil {
			log.Printf("notifyPresencePublishErr: %s\n", err)
			continue
		}
	}
}

func (s *Socket) Register(c *Client) {
	s.Lock()
	s.clients[c.ID] = c
	s.Unlock()
}

func (s *Socket) Deregister(c *Client) {
	s.Lock()
	delete(s.clients, c.ID)
	s.Unlock()
}

func (s *Socket) Broadcast(msg interface{}) {
	s.RLock()
	defer s.RUnlock()

	for _, client := range s.clients {
		client.Write(msg)
	}
}

func (s *Socket) Publish(id string, msg interface{}) {
	s.RLock()
	defer s.RUnlock()

	if client, exist := s.clients[id]; exist {
		client.Write(msg)
	}
}
