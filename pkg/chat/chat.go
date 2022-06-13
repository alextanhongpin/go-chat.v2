package chat

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/alextanhongpin/go-chat.v2/pkg/socketio"
	"github.com/go-redis/redis/v9"
)

var friends = map[string][]string{
	"john":  {"alice", "bob"},
	"alice": {"john"},
	"bob":   {"john"},
}

type authorizer interface {
	Issue(subject string) (string, error)
	Verify(token string) (string, error)
}

type UserID string
type SessionID string

type Chat[T Message] struct {
	redis    *socketio.IORedis[T]
	io       *socketio.IO[T]
	done     chan struct{}
	wg       sync.WaitGroup
	evtCh    chan event
	authz    authorizer
	mu       sync.RWMutex
	sessions map[UserID]map[SessionID]bool
}

func New[T Message](channel string, client *redis.Client, authz authorizer) (*Chat[T], func()) {
	io, closeio := socketio.NewIO[T]()
	ioredis, closeredis := socketio.NewIORedis[T](channel, client)

	c := &Chat[T]{
		redis:    ioredis,
		io:       io,
		done:     make(chan struct{}),
		evtCh:    make(chan event),
		authz:    authz,
		sessions: make(map[UserID]map[SessionID]bool),
	}

	c.loopAsync()

	return c, func() {
		closeio()
		closeredis()
	}
}

func (c *Chat[T]) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	username, err := c.authz.Verify(token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	socket, err, close := c.io.Connect(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	defer close()

	c.evtCh <- Connected{
		username:  username,
		sessionID: socket.ID,
		online:    true,
	}

	defer func() {
		fmt.Println("chat: disconnecting")
		c.evtCh <- Connected{
			username:  username,
			sessionID: socket.ID,
			online:    false,
		}
	}()

	for msg := range socket.Listen() {
		m, ok := any(msg).(Message)
		if !ok {
			panic("invalid message type")
		}
		m.From = username

		fmt.Printf("chat: listen: %+v\n", msg)

		users, _ := friends[username]
		for _, user := range users {
			msg := m
			msg.To = user
			if err := c.emitRemote(msg); err != nil {
				panic(err)
			}
		}
	}
	fmt.Println("chat: closing")
}

func (c *Chat[T]) loop() {
	for {
		select {
		case <-c.done:
			break
		case evt := <-c.evtCh:
			fmt.Println("chat: evtCh", evt)
			c.eventProcessor(evt)
			// Read the remote redis and publish to all local nodes.
		case msg := <-c.redis.Subscribe():
			fmt.Println("chat: redis sub", msg)
			switch m := any(msg).(type) {
			case Message:
				c.emitLocal(m)
			default:
				fmt.Println("chat: unhandled", msg)
				continue
			}
		}
	}
}

func (c *Chat[T]) loopAsync() {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		c.loop()
	}()
}

func (c *Chat[T]) eventProcessor(evt event) {
	switch e := evt.(type) {
	case Connected:
		if e.online {
			c.connected(e)
		} else {
			c.disconnected(e)
		}
	default:
		panic("not handled")
	}
}

func (c *Chat[T]) fetchFriends(username string) {
	users := friends[username]

	statuses := make([]Friend, len(users))
	for i, user := range users {
		statuses[i] = Friend{
			Username: user,
			// TODO: ̧Check over the redis.
			Online: c.isUserOnline(user),
		}
	}

	c.emitLocal(Message{
		From:    username,
		To:      username,
		Type:    MessageTypeFriends,
		Friends: statuses,
	})
}

func (c *Chat[T]) notifyPresence(username string, online bool) {
	users := friends[username]

	for _, user := range users {
		msg := Message{
			From:     username,
			To:       user,
			Type:     MessageTypePresence,
			Presence: &online,
		}

		if err := c.emitRemote(msg); err != nil {
			panic(err)
		}
	}
}

func (c *Chat[T]) connected(evt Connected) {
	uid := UserID(evt.username)
	sid := SessionID(evt.sessionID)

	c.mu.Lock()
	m, found := c.sessions[uid]
	if !found {
		m = make(map[SessionID]bool)
		c.sessions[uid] = m
	}
	m[sid] = true
	c.sessions[uid] = m
	c.mu.Unlock()

	c.fetchFriends(evt.username)
	c.notifyPresence(evt.username, true)
}

func (c *Chat[T]) disconnected(evt Connected) {
	fmt.Println("chat: disconnected", evt)
	uid := UserID(evt.username)
	sid := SessionID(evt.sessionID)

	c.mu.Lock()
	m, found := c.sessions[uid]
	if !found {
		c.mu.Unlock()
		return
	}

	delete(m, sid)
	if len(m) == 0 {
		delete(c.sessions, uid)
	} else {
		c.sessions[uid] = m
	}

	c.mu.Unlock()

	c.notifyPresence(evt.username, c.isUserOnline(evt.username))
}

func (c *Chat[T]) isUserOnline(username string) bool {
	uid := UserID(username)

	c.mu.RLock()
	m, ok := c.sessions[uid]
	c.mu.RUnlock()

	return ok && len(m) > 0
}

func (c *Chat[T]) emitLocal(msg Message) bool {
	uid := UserID(msg.To)

	c.mu.RLock()
	m, ok := c.sessions[uid]
	c.mu.RUnlock()

	if ok {
		for sid := range m {
			c.io.EmitAny(string(sid), msg)
		}
	}

	return ok
}

func (c *Chat[T]) emitRemote(msg any) error {
	return c.redis.Publish(context.Background(), msg)
}
