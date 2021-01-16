package chat

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
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

type Command struct {
	Msg string `json:"msg"`
}

type Chat struct {
	issuer ticket.Issuer
}

func New(issuer ticket.Issuer) *Chat {
	return &Chat{
		issuer: issuer,
	}
}

func (c *Chat) authorize(r *http.Request) (string, error) {
	token := r.URL.Query().Get("token")
	user, err := c.issuer.Verify(token)
	if err != nil {
		return "", err
	}
	return user, nil
}

func (c *Chat) ServeWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgradeWebSocketErr: %s", err)
		return
	}
	defer ws.Close()

	user, err := c.authorize(r)
	if err != nil {
		ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("unauthorized: %s", err.Error())))
		return
	}
	log.Println("connected:", user)

	ch := make(chan Command)
	var wg sync.WaitGroup
	wg.Add(1)

	defer func() {
		log.Println("disconnected:", user)
	}()

	go func() {
		defer wg.Done()
		write(ws, ch)
	}()

	read(ws, ch)

	close(ch)
	wg.Wait()
}

func read(ws *websocket.Conn, ch chan Command) {
	defer ws.Close()

	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	var cmd Command
	for {
		if err := ws.ReadJSON(&cmd); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		ch <- cmd
		fmt.Println("got msg", cmd)
	}
}

func write(ws *websocket.Conn, ch chan Command) {
	defer ws.Close()

	t := time.NewTicker(pingPeriod)
	defer t.Stop()

	for {
		select {
		case cmd, ok := <-ch:
			if !ok {
				// The hub closed the channel.
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteJSON(cmd); err != nil {
				// Unregister.
				return
			}

		// PING.
		case <-t.C:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}
