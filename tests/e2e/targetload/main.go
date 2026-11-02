// Actual disposable RabbitMQ publish/consume/manual-ACK workload.
// Protocol example: https://www.rabbitmq.com/tutorials/tutorial-two-go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	if err := load(); err != nil {
		fmt.Fprintln(os.Stderr, "owned target workload failed")
		os.Exit(1)
	}
}

func load() error {
	password := os.Getenv("RABBITMQ_DEFAULT_PASS")
	if password == "" {
		return errors.New("fixture credential absent")
	}
	endpoint := url.URL{Scheme: "amqp", Host: "127.0.0.1:5672", Path: "/", User: url.UserPassword("bootstrap", password)}
	connection, err := amqp.DialConfig(endpoint.String(), amqp.Config{Heartbeat: 10 * time.Second, Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		return errors.New("fixture broker connection failed")
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	queue, err := channel.QueueDeclare("plan8-owned", true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err = channel.Qos(1, 0, false); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	messages, err := channel.ConsumeWithContext(ctx, queue.Name, "argus-e2e-load", false, false, false, false, nil)
	if err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err = channel.PublishWithContext(ctx, "", queue.Name, false, false, amqp.Publishing{ContentType: "text/plain", Body: []byte("argus-e2e")}); err != nil {
				return err
			}
		case message, ok := <-messages:
			if !ok {
				return errors.New("fixture consumer stopped")
			}
			if err = message.Ack(false); err != nil {
				return err
			}
		}
	}
}
