package socket

import "reflect"

type Event interface {
	GetTypeName() string
}

func getTypeName(i interface{}) string {
	if t := reflect.TypeOf(i); t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	} else {
		return t.Name()
	}
}

type Connected struct {
	TypeName string
	Client   *Client
}

func NewConnectedEvent(c *Client) Connected {
	return Connected{
		TypeName: getTypeName(new(Connected)),
		Client:   c,
	}
}

func (e Connected) GetTypeName() string {
	return e.TypeName
}

type Disconnected struct {
	TypeName string
	Client   *Client
}

func NewDisconnectedEvent(c *Client) Disconnected {
	return Disconnected{
		TypeName: getTypeName(new(Disconnected)),
		Client:   c,
	}
}

func (e Disconnected) GetTypeName() string {
	return e.TypeName
}
