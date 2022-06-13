package socketio

import "github.com/gorilla/websocket"

func NewError(msg string) *SocketError {
	return &SocketError{
		Code:    websocket.ClosePolicyViolation,
		Message: msg,
	}
}

type SocketError struct {
	Code    int
	Message string
}

func (s *SocketError) Error() string {
	return s.Message
}

func writeError(ws *websocket.Conn, code int, err error) {
	_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, err.Error()))
}

func writeClose(ws *websocket.Conn) {
	_ = ws.WriteMessage(websocket.CloseMessage, []byte{})
}

func writePing(ws *websocket.Conn) error {
	return ws.WriteMessage(websocket.PingMessage, []byte{})
}
