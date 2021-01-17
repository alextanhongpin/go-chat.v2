package socket

import (
	"log"
	"sync"
	"time"

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

type Client struct {
	ID   string
	User string

	wg   sync.WaitGroup
	ws   *websocket.Conn
	once sync.Once

	done  chan struct{}
	msgCh chan Message
	errCh chan SocketError
	evtCh chan Event
}

func NewClient(ws *websocket.Conn, user string) *Client {
	return &Client{
		ID:   uuid.New().String(),
		User: user,

		ws: ws,

		done:  make(chan struct{}),
		msgCh: make(chan Message),
		errCh: make(chan SocketError),
		evtCh: make(chan Event, 2), // At most two events.
	}
}

func (c *Client) Connect() {
	c.evtCh <- NewConnectedEvent(c)
	c.write(c.ws)
}

// Close signals the write channel to be closed. Idempotent, can be
// called multiple times.
func (c *Client) Close() {
	c.once.Do(func() {
		c.evtCh <- NewDisconnectedEvent(c)
		close(c.done)
		c.wg.Wait()
	})
}

// Emit sends a message to the client.
func (c *Client) Emit(m Message) {
	c.msgCh <- m
}

// Emit sends a message to the client.
func (c *Client) Error(err SocketError) {
	c.errCh <- err
}

// On listens to messages from the client.
func (c *Client) On() <-chan Message {
	return read(c.ws)
}

func (c *Client) Events() <-chan Event {
	return c.evtCh
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
			case err := <-c.errCh:
				ws.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(err.Code, err.Error()))
				return

			case msg, ok := <-c.msgCh:
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
