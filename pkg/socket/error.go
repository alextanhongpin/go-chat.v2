package socket

import "github.com/gorilla/websocket"

type SocketError struct {
	Code int
	Err  error
}

func (s SocketError) Error() string {
	return s.Err.Error()
}

func InternalServerError(err error) SocketError {
	return SocketError{
		Code: websocket.CloseInternalServerErr,
		Err:  err,
	}
}

func PolicyViolation(err error) SocketError {
	return SocketError{
		Code: websocket.ClosePolicyViolation,
		Err:  err,
	}
}
