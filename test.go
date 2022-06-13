package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alextanhongpin/go-chat.v2/infra"
)

func main() {
	redis := infra.NewRedis()
	ctx := context.Background()
	channel := "my-topic"
	sub := redis.Subscribe(ctx, channel)

	if err := sub.Ping(ctx); err != nil {
		panic(err)
	}
	publish := func(msg string) {
		if err := redis.Publish(ctx, channel, msg).Err(); err != nil {
			fmt.Println("failed to publish:", err)
		}
	}
	go func() {
		publish("hello")
		time.Sleep(1 * time.Second)

		publish("world")
		time.Sleep(5 * time.Second)
		fmt.Println("closing", sub.Close())
	}()

	for msg := range sub.Channel() {
		fmt.Println("received:", msg)
	}
}
