package socket

type Message struct {
	Type    string                 `json:"type"`
	From    string                 `json:"from"`
	To      string                 `json:"to"`
	Owner   bool                   `json:"owner"`
	Payload map[string]interface{} `json:"payload"`
}
