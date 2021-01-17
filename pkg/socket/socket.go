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

const channel = "chat"

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Socket struct {
	sync.RWMutex
	clients  map[string]*Client
	issuer   ticket.Issuer
	rdb      *redis.Client
	evtCh    chan Event
	clientCh chan *ClientListener
}

type ClientListener struct {
	Client *Client
	Chan   chan *Client
}

func NewSocket(issuer ticket.Issuer) (*Socket, func()) {
	s := &Socket{
		clients:  make(map[string]*Client),
		issuer:   issuer,
		rdb:      NewRedis(),
		evtCh:    make(chan Event),
		clientCh: make(chan *ClientListener),
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

func (s *Socket) JoinRoom(room, socketID string) error {
	return s.rdb.SAdd(context.Background(), room, []string{socketID}).Err()
}

func (s *Socket) LeaveRoom(room, socketID string) error {
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

	// Sends the client to the listener while blocking the main loop here.
	// This make use of messaging patterns to keep the code reusable.
	listener := &ClientListener{
		Client: client,
		Chan:   make(chan *Client),
	}
	s.clientCh <- listener
	<-listener.Chan
}

func (s *Socket) Connected(client *Client) {
	s.Register(client)
	s.evtCh <- NewRegisteredEvent(client)
}

func (s *Socket) EventListener() chan Event {
	return s.evtCh
}

func (s *Socket) ClientListener() chan *ClientListener {
	return s.clientCh
}

func (s *Socket) Disconnected(client *Client) {
	s.Deregister(client)
	s.evtCh <- NewDeregisteredEvent(client)
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
