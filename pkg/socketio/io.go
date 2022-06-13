package socketio

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	readBufferSize  = 1024
	writeBufferSize = 1024
)

type IO[T any] struct {
	websocket.Upgrader
	SocketFunc   func(s *Socket[T]) error
	mu           sync.RWMutex
	wg           sync.WaitGroup
	quit         sync.Once
	done         chan struct{}
	registerCh   chan *Socket[T]
	unregisterCh chan *Socket[T]
	sockets      map[string]*Socket[T]
}

func NewIO[T any]() (*IO[T], func()) {
	io := &IO[T]{
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  readBufferSize,
			WriteBufferSize: writeBufferSize,
		},
		done:         make(chan struct{}),
		registerCh:   make(chan *Socket[T]),
		unregisterCh: make(chan *Socket[T]),
		sockets:      make(map[string]*Socket[T]),
	}
	io.loopAsync()

	return io, io.close
}

func (io *IO[T]) Connect(w http.ResponseWriter, r *http.Request) (*Socket[T], error, func()) {
	ws, err := io.Upgrade(w, r, nil)
	if err != nil {
		return nil, fmt.Errorf("io: failed to upgrade websocket connection: %w", err), nil
	}

	socket, _ := NewSocket[T](ws)
	if io.SocketFunc != nil {
		io.SocketFunc(socket)
	}

	io.registerCh <- socket

	return socket, nil, func() {
		io.unregisterCh <- socket
	}
}

func (io *IO[T]) Emit(socketID string, msg T) bool {
	io.mu.RLock()
	socket, ok := io.sockets[socketID]
	io.mu.RUnlock()

	if !ok {
		return ok
	}

	return socket.Emit(msg)
}

func (io *IO[T]) EmitAny(socketID string, msg any) bool {
	io.mu.RLock()
	socket, ok := io.sockets[socketID]
	io.mu.RUnlock()

	if !ok {
		return ok
	}

	return socket.EmitAny(msg)
}

func (io *IO[T]) BroadcastAny(msg any) (total int, sent int) {
	io.mu.Lock()
	total = len(io.sockets)

	for _, socket := range io.sockets {
		if socket.EmitAny(msg) {
			sent++
		}
	}
	io.mu.Unlock()

	return
}

func (io *IO[T]) Broadcast(msg T) (total int, sent int) {
	io.mu.Lock()
	total = len(io.sockets)

	for _, socket := range io.sockets {
		if socket.Emit(msg) {
			sent++
		}
	}
	io.mu.Unlock()

	return
}

func (io *IO[T]) close() {
	io.quit.Do(func() {
		close(io.done)
		io.wg.Wait()
	})
}

func (io *IO[T]) loop() {
	defer func() {
		close(io.registerCh)
		close(io.unregisterCh)
	}()

	for {
		select {
		case <-io.done:
			return
		case socket := <-io.registerCh:
			io.mu.Lock()
			io.sockets[socket.ID] = socket
			io.mu.Unlock()
		case socket := <-io.unregisterCh:
			io.mu.Lock()
			socket, ok := io.sockets[socket.ID]
			if ok {
				socket.close()
				delete(io.sockets, socket.ID)
			}
			io.mu.Unlock()
		}
	}
}

func (io *IO[T]) loopAsync() {
	io.wg.Add(1)

	go func() {
		defer io.wg.Done()

		io.loop()
	}()
}
