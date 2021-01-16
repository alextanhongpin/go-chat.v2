package socket

import (
	"log"
	"sync"
	"time"

	"github.com/alextanhongpin/go-chat.v2/pkg/eventbus"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	channel = "chat"

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

type Client struct {
	ID string
	ch chan interface{}
	eventbus.Engine
}

func NewClient() *Client {
	return &Client{
		ID:     uuid.New().String(),
		ch:     make(chan interface{}),
		Engine: eventbus.New(),
	}
}

func (c *Client) ServeWS(ws *websocket.Conn) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		c.write(ws)
	}()
	c.read(ws)

	close(c.ch)
	wg.Wait()
}

func (c *Client) read(ws *websocket.Conn) {
	defer ws.Close()

	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	var msg Message
	for {
		if err := ws.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		if err := c.Emit(msg.Type, msg.Payload); err != nil {
			ws.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, err.Error()))
			return
		}
	}
}

func (c *Client) Write(in interface{}) {
	c.ch <- in
}

func (c *Client) write(ws *websocket.Conn) {
	defer ws.Close()

	t := time.NewTicker(pingPeriod)
	defer t.Stop()

	for {
		select {
		case msg, ok := <-c.ch:
			if !ok {
				// The hub closed the channel.
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteJSON(msg); err != nil {
				log.Printf("writeJSONErr: %s\n", err)
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
