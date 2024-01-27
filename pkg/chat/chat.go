package chat

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/alextanhongpin/go-chat.v2/pkg/socketio"
	"github.com/redis/go-redis/v9"
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

type Chat struct {
	remote *socketio.IORedis[Message]
	local  *socketio.IO[Message]
	wg     sync.WaitGroup
	done   chan struct{}
	evtCh  chan event
	authz  authorizer
	mu     sync.RWMutex
}

func New(channel string, client *redis.Client, authz authorizer) (*Chat, func()) {
	io := socketio.NewIO[Message]()
	ioredis, close := socketio.NewIORedis[Message](channel, client)

	chat := &Chat{
		remote: ioredis,
		local:  io,
		done:   make(chan struct{}),
		evtCh:  make(chan event),
		authz:  authz,
	}

	chat.loopAsync()

	return chat, close
}

func (c *Chat) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	username, err := c.authz.Verify(token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)

		return
	}

	socket, err, flush := c.local.ServeWS(w, r)
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
		msg.From = username

		users, _ := friends[username]
		for _, user := range users {
			m := msg
			m.To = user

			if err := c.emitRemote(m); err != nil {
				panic(err)
			}
		}
	}
}

func (c *Chat) loop() {
	ch, stop := c.remote.Subscribe()
	defer stop()

	for {
		select {
		case <-c.done:
			return
		case evt := <-c.evtCh:
			c.eventProcessor(evt)
		case msg := <-ch:
			c.emitLocal(msg)
		}
	}
}

func (c *Chat) loopAsync() {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		c.loop()
	}()
}

func (c *Chat) eventProcessor(evt event) {
	switch e := evt.(type) {
	case Connected:
		c.connected(e)
	case Disconnected:
		c.disconnected(e)
	default:
		panic(fmt.Errorf("chat: unhandled event processor: %+v", evt))
	}
}

func (c *Chat) addSession(username, sessionID string) error {
	return c.remote.Client.SAdd(context.Background(), username, []string{sessionID}).Err()
}

func (c *Chat) removeSession(username, sessionID string) error {
	return c.remote.Client.SRem(context.Background(), username, []string{sessionID}).Err()
}

func (c *Chat) getSessions(username string) ([]string, error) {
	return c.remote.Client.SMembers(context.Background(), username).Result()
}

func (c *Chat) fetchFriends(username string) {
	users := friends[username]

	statuses := make([]Friend, len(users))
	for i, user := range users {
		statuses[i] = Friend{
			Username: user,
			Online:   c.isUserOnline(user),
		}
	}

	c.emitLocal(Message{
		From:    username,
		To:      username,
		Type:    MessageTypeFriends,
		Friends: statuses,
	})
}

func (c *Chat) notifyPresence(username string, online bool) {
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

func (c *Chat) connected(evt Connected) {
	if err := c.addSession(evt.username, evt.sessionID); err != nil {
		c.emitError(evt.sessionID, err)

		return
	}

	c.fetchFriends(evt.username)
	c.notifyPresence(evt.username, true)
}

func (c *Chat) disconnected(evt Disconnected) {
	if err := c.removeSession(evt.username, evt.sessionID); err != nil {
		c.emitError(evt.sessionID, err)

		return
	}

	c.notifyPresence(evt.username, c.isUserOnline(evt.username))
}

func (c *Chat) isUserOnline(username string) bool {
	sessions, err := c.getSessions(username)
	return err == nil && len(sessions) > 0
}

func (c *Chat) emitError(sessionID string, err error) bool {
	return c.local.Error(sessionID, err)
}

func (c *Chat) emitLocal(msg Message) bool {
	sessionIDs, _ := c.getSessions(msg.To)

	for _, sid := range sessionIDs {
		c.local.EmitAny(sid, msg)
	}

	return true
}

func (c *Chat) emitRemote(msg any) error {
	return c.remote.Publish(context.Background(), msg)
}
