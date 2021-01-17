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

type Registered struct {
	TypeName string
	Client   *Client
}

func NewRegisteredEvent(c *Client) Registered {
	return Registered{
		TypeName: getTypeName(new(Registered)),
		Client:   c,
	}
}

func (e Registered) GetTypeName() string {
	return e.TypeName
}

type Deregistered struct {
	TypeName string
	Client   *Client
}

func NewDeregisteredEvent(c *Client) Deregistered {
	return Deregistered{
		TypeName: getTypeName(new(Deregistered)),
		Client:   c,
	}
}

func (e Deregistered) GetTypeName() string {
	return e.TypeName
}
