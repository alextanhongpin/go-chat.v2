package socket

import (
	"log"
	"sync"
	"time"

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
	ID   string
	User string

	wg   sync.WaitGroup
	ws   *websocket.Conn
	done chan struct{}
	once sync.Once

	// Message that is written to the client.
	emitCh chan Message
}

func NewClient(ws *websocket.Conn, user string) *Client {
	c := &Client{
		ws:     ws,
		User:   user,
		ID:     uuid.New().String(),
		emitCh: make(chan Message),
		done:   make(chan struct{}),
	}
	c.init(ws)
	return c
}

func (c *Client) init(ws *websocket.Conn) {
	c.write(ws)
}

// Close signals the write channel to be closed. Idempotent, can be
// called multiple times.
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		c.wg.Wait()
	})
}

// Emit sends a message to the client.
func (c *Client) Emit(m Message) {
	c.emitCh <- m
}

// On listens to messages from the client.
func (c *Client) On() <-chan Message {
	return read(c.ws)
}

func (c *Client) write(ws *websocket.Conn) {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		defer ws.Close()

		t := time.NewTicker(pingPeriod)
		defer t.Stop()

		for {
			select {
			case <-c.done:
				return
			case msg, ok := <-c.emitCh:
				if !ok {
					// The hub closed the channel.
					ws.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}

				ws.SetWriteDeadline(time.Now().Add(writeWait))
				if err := ws.WriteJSON(msg); err != nil {
					ws.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
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
	}()
}

func read(ws *websocket.Conn) <-chan Message {
	ch := make(chan Message)

	go func() {
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
					log.Printf("websocketCloseErr: %s\n", err)
				}
				close(ch)
				return
			}
			ch <- msg
		}
	}()

	return ch
}
