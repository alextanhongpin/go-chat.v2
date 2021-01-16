package socket

type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}
