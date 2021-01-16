package eventbus

import (
	"sync"
)

type Engine interface {
	On(event string, fn EventHandler)
	Emit(event string, in map[string]interface{}) error
}

type EventHandler func(map[string]interface{}) error

type EventBus struct {
	rw       sync.RWMutex
	handlers map[string]EventHandler
}

func New() *EventBus {
	return &EventBus{
		handlers: make(map[string]EventHandler),
	}
}
func (e *EventBus) On(event string, fn EventHandler) {
	e.rw.RLock()
	_, exist := e.handlers[event]
	e.rw.RUnlock()
	if exist {
		return
	}

	e.rw.Lock()
	e.handlers[event] = fn
	e.rw.Unlock()
}

func (e *EventBus) Emit(event string, i map[string]interface{}) error {
	e.rw.RLock()
	defer e.rw.RUnlock()

	if handler, exist := e.handlers[event]; !exist {
		return nil
	} else {
		return handler(i)
	}
}
