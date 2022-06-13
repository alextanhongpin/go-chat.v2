package chat

type event interface {
	isEvent()
}

type Event struct{}

type Connected struct {
	username  string
	sessionID string
}

func (Connected) isEvent() {}

type Disconnected struct {
	username  string
	sessionID string
}

func (Disconnected) isEvent() {}
