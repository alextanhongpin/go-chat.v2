package socketio

type SocketError struct {
	Code    int
	Message string
}

func (s *SocketError) Error() string {
	return s.Message
}
