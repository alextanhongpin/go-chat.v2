package socket

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alextanhongpin/go-chat.v2/pkg/broker"
	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Socket struct {
	broker.Engine
	issuer ticket.Issuer
	rdb    *redis.Client
}

func NewSocket(issuer ticket.Issuer) *Socket {
	return &Socket{
		Engine: broker.New(),
		issuer: issuer,
		rdb:    NewRedis(),
	}
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
	return s.rdb.SAdd(context.Background(), room, socketID).Err()
}

func (s *Socket) LeaveRoom(socketID, room string) error {
	return s.rdb.SRem(context.Background(), room, socketID).Err()
}

func (s *Socket) RoomMembers(room string) ([]string, error) {
	return s.rdb.SMembers(context.Background(), room).Result()
}

func (skt *Socket) ServeWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgradeWebSocketErr: %s", err)
		return
	}

	// Authorize.
	user, err := skt.authorize(r)
	if err != nil {
		ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("unauthorized: %s", err.Error())))
		return
	}

	id := uuid.New()
	ch := skt.Subscribe(id.String())
	defer skt.Unsubscribe(id.String(), ch)
	skt.JoinRoom(id.String(), user)
	defer skt.LeaveRoom(id.String(), user)

	client := NewClient(id, ch, ws)
	client.On("send_message", func(in map[string]interface{}) error {
		client.Write(Message{
			Type: "message_sent",
			Payload: map[string]interface{}{
				"msg":  in["msg"].(string),
				"from": user,
				"to":   user,
			},
		})
		//skt.Broadcast(in)
		return nil
	})

	log.Println("connected:", id)
	client.ServeWS()
	log.Println("disconnected:", id)
}
