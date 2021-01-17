package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

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

func NewSocket(issuer ticket.Issuer) (*Socket, func()) {
	s := &Socket{
		clients: make(map[string]*Client),
		issuer:  issuer,
		rdb:     NewRedis(),
		friends: NewFriendService(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.subscribe(ctx)
	return s, cancel
}

func (s *Socket) subscribe(ctx context.Context) {
	pubsub := s.rdb.Subscribe(ctx, channel)

	go func() {
		ch := pubsub.Channel()

		for msg := range ch {
			var m Message
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				log.Printf("unmarshalErr: %s\n", err)
				continue
			}
			socketIDs, err := s.RoomMembers(m.To)
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

func (s *Socket) CheckOnline(room string) bool {
	members, err := s.RoomMembers(room)
	if err != nil {
		return false
	}
	return len(members) > 0
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
			websocket.FormatCloseMessage(
				websocket.ClosePolicyViolation,
				fmt.Sprintf("unauthorized: %s", err.Error()),
			),
		)
		return
	}

	client := NewClient(ws, user)

	// Close the write channel once the read is closed.
	defer client.Close()

	// Perform logic on connection.
	s.Connected(client)

	// Cleanup.
	defer s.Disconnected(client)

	// Start the read channel, when the user closes the tab, the for loop will
	// stop.
	for msg := range client.On() {
		switch msg.Type {
		case SendMessage:
			msg.From = user
			msg.To = user
			msg.Type = "message_sent"
			s.PublishRemote(context.Background(), msg)
		default:
			log.Printf("not implemented: %v\n", msg)
		}
	}
}

func (s *Socket) Connected(client *Client) {
	s.Register(client)

	if err := s.JoinRoom(client.ID, client.User); err != nil {
		log.Printf("joinRoomErr: %s\n", err)
	}

	if err := s.FetchFriendsStatus(client.User); err != nil {
		log.Printf("friendStatusErr: %s\n", err)
	}

	// NOTE: This is redundant if the user is already online on other devices.
	// A better way is to send if the user logins only on one device.
	s.NotifyPresence(client.User, true)
}

func (s *Socket) Disconnected(client *Client) {
	s.Deregister(client)

	// NOTE: We need a better way to manage user rooms, as they might not be
	// removed from the room, leading to a lot of failed deliveries.
	if err := s.LeaveRoom(client.ID, client.User); err != nil {
		log.Printf("leaveRoomErr: %s\n", err)
	}

	// NOTE: The user may be online on several devices. Even though the user
	// shares the same id, the session is different. We only notify the user is
	// offline when the session counts falls to 0.
	s.NotifyPresence(client.User, s.CheckOnline(client.User))
}

type FriendStatus struct {
	Username string `json:"username"`
	To       string `json:"to"`
	Online   bool   `json:"online"`
}

func (s *Socket) FetchFriendsStatus(user string) error {
	friends := s.friends.FindFriendsFor(user)
	var friendsStatus []FriendStatus
	for _, friend := range friends {
		friendsStatus = append(friendsStatus, FriendStatus{
			Username: friend,
			To:       user,
			Online:   s.CheckOnline(friend),
		})
	}

	msg := Message{
		Type: "friends_fetched",
		To:   user,
		From: user,
		Payload: map[string]interface{}{
			"friends": friendsStatus,
		},
	}
	return s.PublishRemote(context.Background(), msg)
}

func (s *Socket) NotifyPresence(user string, online bool) {
	friends := s.friends.FindFriendsFor(user)

	for _, friend := range friends {
		if !s.CheckOnline(friend) {
			continue
		}

		msg := Message{
			Type: "presence_notified",
			To:   friend,
			From: user,
			Payload: map[string]interface{}{
				"username": user,
				"online":   online,
			},
		}

		if err := s.PublishRemote(context.Background(), msg); err != nil {
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

func (s *Socket) Broadcast(msg Message) {
	s.RLock()
	defer s.RUnlock()

	for _, client := range s.clients {
		client.Emit(msg)
	}
}

func (s *Socket) Publish(id string, msg Message) {
	s.RLock()
	defer s.RUnlock()

	if client, exist := s.clients[id]; exist {
		client.Emit(msg)
	}
}

func (s *Socket) PublishRemote(ctx context.Context, in interface{}) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, channel, string(b)).Err()
}
