package socketio

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (

	// Time allowed to write a message to the peer.
	writeTimeout = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongTimeout = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingTimeout = (pongTimeout * 9) / 10

	maxMessageSize = 512
)

type Socket[T any] struct {
	ID             string
	WriteTimeout   time.Duration
	PingTimeout    time.Duration
	PongTimeout    time.Duration
	MaxMessageSize int64

	conn    *websocket.Conn
	done    chan struct{}
	quit    sync.Once
	wg      sync.WaitGroup
	readCh  chan T
	writeCh chan any
}

func NewSocket[T any](conn *websocket.Conn) (*Socket[T], func()) {
	socket := &Socket[T]{
		ID:             uuid.New().String(),
		WriteTimeout:   writeTimeout,
		PongTimeout:    pongTimeout,
		PingTimeout:    pingTimeout,
		MaxMessageSize: maxMessageSize,

		conn:    conn,
		done:    make(chan struct{}),
		readCh:  make(chan T),
		writeCh: make(chan any),
	}

	socket.wg.Add(1)
	go func() {
		defer socket.wg.Done()

		fmt.Println("socket: init writer")
		socket.writer()
	}()

	socket.wg.Add(1)

	go func() {
		defer socket.wg.Done()

		fmt.Println("socket: init reader at listen")
		socket.reader()
	}()

	return socket, socket.close
}

func (s *Socket[T]) Emit(msg T) bool {
	select {
	case <-s.done:
		return false
	case s.writeCh <- msg:
		return true
	}
}

func (s *Socket[T]) EmitAny(msg any) bool {
	select {
	case <-s.done:
		return false
	case s.writeCh <- msg:
		return true
	}
}

func (s *Socket[T]) Listen() <-chan T {

	return s.readCh
}

func (s *Socket[T]) close() {
	s.quit.Do(func() {
		close(s.done)
		s.wg.Wait()
		s.conn.Close()
	})
}

func (s *Socket[T]) writer() {
	pinger := time.NewTicker(s.PingTimeout)
	defer pinger.Stop()

	for {
		select {
		case <-s.done:
			return
		case msg, open := <-s.writeCh:
			if !open {
				_ = s.conn.WriteMessage(websocket.CloseMessage, []byte{})

				return
			}
			fmt.Println("socket: writing", msg)

			_ = s.conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
			if err := s.conn.WriteJSON(msg); err != nil {
				_ = s.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))

				return
			}

		case <-pinger.C:
			fmt.Println("socket: ping")
			_ = s.conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout))

			if err := s.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (s *Socket[T]) reader() {
	for {
		s.conn.SetReadLimit(s.MaxMessageSize)
		_ = s.conn.SetReadDeadline(time.Now().Add(s.PongTimeout))
		s.conn.SetPongHandler(func(string) error {
			return s.conn.SetReadDeadline(time.Now().Add(s.PongTimeout))
		})

		var msg T
		if err := s.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("socket: %s\n", err)
			}

			return
		}
		fmt.Println("socket: read", msg)

		select {
		case <-s.done:
			return
		case s.readCh <- msg:
		}
	}
}
