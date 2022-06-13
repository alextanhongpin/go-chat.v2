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

type Chat[T Message] struct {
	redis *socketio.IORedis[T]
	io    *socketio.IO[T]
	done  chan struct{}
	wg    sync.WaitGroup
	evtCh chan event
	authz authorizer
	mu    sync.RWMutex
}

func New[T Message](channel string, client *redis.Client, authz authorizer) (*Chat[T], func()) {
	io, closeio := socketio.NewIO[T]()
	ioredis, closeredis := socketio.NewIORedis[T](channel, client)

	chat := &Chat[T]{
		redis: ioredis,
		io:    io,
		done:  make(chan struct{}),
		evtCh: make(chan event),
		authz: authz,
	}

	chat.loopAsync()

	return chat, func() {
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

	socket, err, flush := c.io.ServeWS(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	defer flush()

	c.evtCh <- Connected{
		username:  username,
		sessionID: socket.ID,
	}

	defer func() {
		c.evtCh <- Disconnected{
			username:  username,
			sessionID: socket.ID,
		}
	}()

	for msg := range socket.Listen() {
		m, ok := any(msg).(Message)
		if !ok {
			panic("invalid message type")
		}
		m.From = username

		users, _ := friends[username]
		for _, user := range users {
			msg := m
			msg.To = user
			if err := c.emitRemote(msg); err != nil {
				panic(err)
			}
		}
	}
}

func (c *Chat[T]) loop() {
	for {
		select {
		case <-c.done:
			return

		case evt := <-c.evtCh:
			c.eventProcessor(evt)
			// Read the remote redis and publish to all local nodes.
		case msg := <-c.redis.Subscribe():
			switch m := any(msg).(type) {
			case Message:
				c.emitLocal(m)
			default:
				panic(fmt.Errorf("chat: unhandled message subscription: %+v", msg))
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
		c.connected(e)
	case Disconnected:
		c.disconnected(e)
	default:
		panic(fmt.Errorf("chat: unhandled event processor: %+v", evt))
	}
}

func (c *Chat[T]) addSession(username, sessionID string) error {
	return c.redis.Client.SAdd(context.Background(), username, []string{sessionID}).Err()
}

func (c *Chat[T]) removeSession(username, sessionID string) error {
	return c.redis.Client.SRem(context.Background(), username, []string{sessionID}).Err()
}

func (c *Chat[T]) getSessions(username string) ([]string, error) {
	return c.redis.Client.SMembers(context.Background(), username).Result()
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
	if err := c.addSession(evt.username, evt.sessionID); err != nil {
		c.emitError(evt.sessionID, err)

		return
	}

	c.fetchFriends(evt.username)
	c.notifyPresence(evt.username, true)
}

func (c *Chat[T]) disconnected(evt Disconnected) {
	if err := c.removeSession(evt.username, evt.sessionID); err != nil {
		c.emitError(evt.sessionID, err)

		return
	}

	c.notifyPresence(evt.username, c.isUserOnline(evt.username))
}

func (c *Chat[T]) isUserOnline(username string) bool {
	sessions, err := c.getSessions(username)
	return err == nil && len(sessions) > 0
}

func (c *Chat[T]) emitError(sessionID string, err error) bool {
	return c.io.EmitAny(sessionID, err)
}

func (c *Chat[T]) emitLocal(msg Message) bool {
	sessionIDs, _ := c.getSessions(msg.To)

	for _, sid := range sessionIDs {
		c.io.EmitAny(sid, msg)
	}

	return true
}

func (c *Chat[T]) emitRemote(msg any) error {
	return c.redis.Publish(context.Background(), msg)
}
