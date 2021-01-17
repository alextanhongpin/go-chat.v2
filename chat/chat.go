package chat

import (
	"context"
	"log"
	"sync"

	"github.com/alextanhongpin/go-chat.v2/pkg/socket"
	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
	"github.com/alextanhongpin/go-chat.v2/usecase"
)

const (
	SendMessage = "send_message"
)

type Chat struct {
	wg sync.WaitGroup

	*socket.Socket
	friends *usecase.FriendService
}

func New(issuer ticket.Issuer) *Chat {
	c := &Chat{
		Socket:  socket.NewSocket(issuer),
		friends: usecase.NewFriendService(),
	}
	c.init()
	return c
}

func (c *Chat) init() {
	c.initEventListener()
}

func (c *Chat) Close() {
	c.Socket.Close()
	c.wg.Wait()
}

func (c *Chat) initEventListener() {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		for evt := range c.EventListener() {
			c.eventProcessor(evt)
		}
	}()
}

func (c *Chat) eventProcessor(evt socket.Event) {
	switch e := evt.(type) {
	case socket.Connected:
		c.connected(e)
	case socket.Disconnected:
		c.disconnected(e)
	default:
		log.Printf("not handle: %s\n", e.GetTypeName())
	}
}

func (c *Chat) connected(e socket.Connected) {
	log.Println("connected", e.Client.User)
	var (
		client = e.Client
		user   = client.User
		id     = client.ID
	)
	if err := c.JoinRoom(user, id); err != nil {
		client.Error(socket.InternalServerError(err))
		return
	}
	log.Println("joined room", e.Client.User)

	if err := c.fetchFriendsStatus(user); err != nil {
		client.Error(socket.InternalServerError(err))
		return
	}

	// NOTE: This is redundant if the user is already online on other devices.
	// A better way is to send if the user logins only on one device.
	c.notifyPresence(user, true)
}

func (c *Chat) disconnected(e socket.Disconnected) {
	var (
		client = e.Client
		user   = client.User
		id     = client.ID
	)
	log.Println("disconnected", e.Client.User)

	if err := c.LeaveRoom(user, id); err != nil {
		client.Error(socket.InternalServerError(err))
		return
	}
	log.Println("left room", e.Client.User)

	// NOTE: The user may be online on several devices. Even though the user
	// shares the same id, the session is different. We only notify the user is
	// offline when the session counts falls to 0.
	c.notifyPresence(user, c.CheckOnline(user))
}

type FriendStatus struct {
	Username string `json:"username"`
	To       string `json:"to"`
	Online   bool   `json:"online"`
}

func (c *Chat) fetchFriendsStatus(user string) error {
	friends := c.friends.FindFriendsFor(user)

	var friendsStatus []FriendStatus
	for _, friend := range friends {
		friendsStatus = append(friendsStatus, FriendStatus{
			Username: friend,
			To:       user,
			Online:   c.CheckOnline(friend),
		})
	}

	msg := socket.Message{
		Type: "friends_fetched",
		To:   user,
		From: user,
		Payload: map[string]interface{}{
			"friends": friendsStatus,
		},
	}
	return c.PublishRemote(context.Background(), msg)
}

func (c *Chat) notifyPresence(user string, online bool) {
	friends := c.friends.FindFriendsFor(user)

	for _, friend := range friends {
		if !c.CheckOnline(friend) {
			continue
		}

		msg := socket.Message{
			Type: "presence_notified",
			To:   friend,
			From: user,
			Payload: map[string]interface{}{
				"username": user,
				"online":   online,
			},
		}

		if err := c.PublishRemote(context.Background(), msg); err != nil {
			log.Printf("notifyPresencePublishErr: %s\n", err)
			continue
		}
	}
}
