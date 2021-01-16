package socket

import (
	"log"
	"sync"
	"time"

	"github.com/alextanhongpin/go-chat.v2/pkg/eventbus"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	id uuid.UUID
	ch chan interface{}
	ws *websocket.Conn
	eventbus.Engine
}

func NewClient(id uuid.UUID, ch chan interface{}, ws *websocket.Conn) *Client {
	return &Client{
		id:     id,
		ch:     ch,
		ws:     ws,
		Engine: eventbus.New(),
	}
}

func (c *Client) ServeWS() {
	ws := c.ws

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
