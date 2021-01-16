package socket

// TODO: Cleanup message format, need from and to, as well as timestamp.
type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}
