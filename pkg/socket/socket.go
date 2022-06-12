package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
	"github.com/go-redis/redis/v9"
	"github.com/gorilla/websocket"
)

const channel = "chat"

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Socket struct {
	sync.RWMutex
	wg      sync.WaitGroup
	clients map[string]*Client
	issuer  ticket.Issuer
	rdb     *redis.Client
	once    sync.Once
	done    chan struct{}
	evtCh   chan Event
}

func NewSocket(issuer ticket.Issuer) *Socket {
	s := &Socket{
		clients: make(map[string]*Client),
		issuer:  issuer,
		rdb:     NewRedis(),
		done:    make(chan struct{}),
		evtCh:   make(chan Event),
	}
	s.init()

	return s
}

func (s *Socket) init() {
	s.subscribe()
}

func (s *Socket) Close() {
	s.once.Do(func() {
		close(s.done)
		close(s.evtCh)
		s.wg.Wait()
	})
}

func (s *Socket) subscribe() {
	s.wg.Add(1)
	pubsub := s.rdb.Subscribe(context.Background(), channel)

	go func() {
		defer s.wg.Done()

		ch := pubsub.Channel()

		for {
			select {
			case <-s.done:
				return
			case msg := <-ch:
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

func (s *Socket) Connect(w http.ResponseWriter, r *http.Request) (*Client, error) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	client := NewClient(ws, user)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		for evt := range client.Events() {
			switch e := evt.(type) {
			case Connected:
				s.register(e.Client)
				s.evtCh <- evt
			case Disconnected:
				s.deregister(e.Client)
				s.evtCh <- evt
			}
		}
	}()

	client.Connect()
	return client, nil
}

func (s *Socket) EventListener() chan Event {
	return s.evtCh
}

func (s *Socket) register(c *Client) {
	s.Lock()
	s.clients[c.ID] = c
	s.Unlock()
}

func (s *Socket) deregister(c *Client) {
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
