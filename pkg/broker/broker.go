package broker

import (
	"sync"
)

type Engine interface {
	Subscribe(topic string) Subscriber
	Unsubscribe(topic string, ch Subscriber)
	Publish(topic string, msg interface{})
	Broadcast(msg interface{})
	HasTopic(topic string) bool
}

type Subscriber chan interface{}

type Topic map[Subscriber]struct{}

type Broker struct {
	rw sync.RWMutex

	// topics holds a map of topic name to Topic.
	topics map[string]Topic
}

func New() *Broker {
	return &Broker{
		topics: make(map[string]Topic),
	}
}

func (b *Broker) HasTopic(name string) bool {
	b.rw.RLock()
	_, exists := b.topics[name]
	b.rw.RUnlock()
	return exists
}

func (b *Broker) createTopic(name string) {
	if b.HasTopic(name) {
		return
	}

	b.rw.Lock()
	b.topics[name] = make(Topic)
	b.rw.Unlock()
}

func (b *Broker) Subscribe(topic string) Subscriber {
	b.createTopic(topic)

	b.rw.Lock()
	ch := make(chan interface{})
	b.topics[topic][ch] = struct{}{}
	b.rw.Unlock()

	return ch
}

func (b *Broker) Unsubscribe(topic string, ch Subscriber) {
	if !b.HasTopic(topic) {
		return
	}

	b.rw.Lock()
	delete(b.topics[topic], ch)
	if len(b.topics[topic]) == 0 {
		delete(b.topics, topic)
	}
	b.rw.Unlock()
}

func (b *Broker) Publish(topic string, msg interface{}) {
	if !b.HasTopic(topic) {
		return
	}

	b.rw.RLock()
	topics := b.topics[topic]
	b.rw.RUnlock()

	for subscriber := range topics {
		subscriber <- msg
	}
}

func (b *Broker) Broadcast(msg interface{}) {
	b.rw.RLock()
	for _, topic := range b.topics {
		for subscriber := range topic {
			subscriber <- msg
		}
	}
	b.rw.RUnlock()
}
