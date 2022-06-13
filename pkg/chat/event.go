package chat

type event interface {
	isEvent()
}

type Event struct{}

type Connected struct {
	username  string
	sessionID string
	online    bool
}

func (Connected) isEvent() {}
