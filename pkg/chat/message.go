package chat

type MessageType string

var (
	MessageTypeText     MessageType = "text"
	MessageTypePresence MessageType = "presence"
	MessageTypeFriends  MessageType = "friends"
)

type Message struct {
	From     string      `json:"from"`
	To       string      `json:"to"`
	Type     MessageType `json:"type"`
	Text     string      `json:"text,omitempty"`
	Presence *bool       `json:"presence,omitempty"`
	Friends  []Friend    `json:"friends,omitempty"`
}

type Friend struct {
	Username string `json:"username"`
	Online   bool   `json:"online"`
}
