package socketio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

type IORedis[T any] struct {
	Client  *redis.Client
	wg      sync.WaitGroup
	channel string
	done    chan struct{}
	quit    sync.Once
}

func NewIORedis[T any](channel string, client *redis.Client) (*IORedis[T], func()) {
	io := &IORedis[T]{
		Client:  client,
		channel: channel,
		done:    make(chan struct{}),
	}
	io.subscribeAsync()

	return io, io.close
}

func (io *IORedis[T]) Publish(ctx context.Context, msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("ioredis: failed to marshal: %w", err)
	}

	if err := io.Client.Publish(ctx, io.channel, string(b)).Err(); err != nil {
		return fmt.Errorf("ioredis: failed to publish: %w", err)
	}

	return nil
}

func (io *IORedis[T]) Subscribe() (<-chan T, func()) {
	ch := io.subscribeAsync()

	var once sync.Once

	close := func() {
		once.Do(func() {
			close(ch)
		})
	}

	return ch, close
}

func (io *IORedis[T]) close() {
	io.quit.Do(func() {
		close(io.done)
		io.wg.Wait()
	})
}

func (io *IORedis[T]) subscribeAsync() chan T {
	ch := make(chan T)

	io.wg.Add(1)

	go func() {
		defer io.wg.Done()

		io.subscribe(ch)
	}()

	return ch
}

func (io *IORedis[T]) subscribe(ch chan<- T) {
	ctx := context.Background()

	pubsub := io.Client.Subscribe(ctx, io.channel)
	defer pubsub.Close()

	for {
		select {
		case <-io.done:
			return
		case data, ok := <-pubsub.Channel():
			if !ok {
				log.Printf("ioredis: channel closed\n")
			}

			var msg T
			if err := json.Unmarshal([]byte(data.Payload), &msg); err != nil {
				log.Printf("ioredis: unmarshal error: %s\n", err)

				continue
			}

			select {
			case <-io.done:
				return
			case ch <- msg:
			}
		}
	}
}
