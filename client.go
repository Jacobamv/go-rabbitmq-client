package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	amqp "github.com/rabbitmq/amqp091-go"
)

type provider struct {
	Address         string
	Cache           *cache.Cache
	Connection      *amqp.Connection
	Channel         *amqp.Channel
	Consumers       map[string]func([]byte) error
	ReconnectTicker *time.Ticker

	consumerCtx    context.Context
	consumerCancel context.CancelFunc
	mutex          sync.Mutex // Prevent multiple goroutines reconnecting
}

type Client interface {
	Run()
	Close() error
	Consume(queueName string, consumer func([]byte) error)
	Send(messageData []byte, queueName string) error
}

func (p *provider) Connect(rmqpAddress string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.Connection != nil {
		_ = p.Connection.Close()
	}

	conn, err := amqp.Dial(rmqpAddress)
	if err != nil {
		return err
	}

	p.Connection = conn
	p.Address = rmqpAddress

	p.Connection.Config.Heartbeat = 15 * time.Minute

	if err := p.NewChannel(); err != nil {
		return err
	}

	go p.AutoReconnect()

	return nil
}

func (p *provider) AutoReconnect() {
	for {
		errChan := p.Connection.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case reason := <-errChan:
			if reason != nil {
				fmt.Println("RabbitMQ connection lost:", reason.Reason)
				for {
					time.Sleep(3 * time.Second)
					fmt.Println("Attempting to reconnect to RabbitMQ...")

					if err := p.Connect(p.Address); err == nil {
						fmt.Println("Reconnected successfully to RabbitMQ")
						p.Run()
						return
					}
				}
			}
		}
	}
}

func (p *provider) NewChannel() error {
	if p.Channel != nil {
		_ = p.Channel.Close()
	}

	ch, err := p.Connection.Channel()
	if err != nil {
		return err
	}

	p.Channel = ch
	return nil
}

func (p *provider) Consume(queueName string, consumer func([]byte) error) {
	p.Consumers[queueName] = consumer
}

func (p *provider) Run() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Stop existing consumers before restarting
	if p.consumerCancel != nil {
		p.consumerCancel()
	}

	p.consumerCtx, p.consumerCancel = context.WithCancel(context.Background())

	for queueName, consumerFunc := range p.Consumers {
		go p.startConsumer(queueName, consumerFunc)
	}

	// Ticker for restarting RabbitMQ connection and consumers every 10 minutes
	go func() {
		for range p.ReconnectTicker.C {
			fmt.Println("Restarting RabbitMQ connection and consumers")

			if err := p.NewChannel(); err != nil {
				log.Printf("Failed to create a new channel: %v", err)
				continue
			}

			p.Run() // Restart consumers
		}
	}()
}

func (p *provider) startConsumer(queueName string, consumerFunc func([]byte) error) {
	q, err := p.Channel.QueueDeclare(
		queueName, false, false, false, false, nil,
	)
	if err != nil {
		log.Printf("Failed to declare queue %s: %v", queueName, err)
		return
	}

	msgs, err := p.Channel.Consume(
		q.Name, "", true, false, false, false, nil,
	)
	if err != nil {
		log.Printf("Failed to register consumer for queue %s: %v", queueName, err)
		return
	}

	fmt.Println("Listening on queue:", queueName)

	for {
		select {
		case <-p.consumerCtx.Done():
			fmt.Println("Stopping consumer for", queueName)
			return
		case msg, ok := <-msgs:
			if !ok {
				fmt.Println("Consumer channel closed for", queueName)
				return
			}

			if err := consumerFunc(msg.Body); err != nil {
				_ = republishMessage(p.Channel, msg, queueName, err)
			}
		}
	}
}

func (p *provider) Send(messageData []byte, queueName string) error {
	q, err := p.Channel.QueueDeclare(queueName, false, false, false, false, nil)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return p.Channel.PublishWithContext(ctx, "", q.Name, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        messageData,
	})
}

func republishMessage(ch *amqp.Channel, msg amqp.Delivery, queueName string, err error) error {
	q, err2 := ch.QueueDeclare("error_"+queueName, false, false, false, false, nil)
	if err2 != nil {
		return err2
	}

	return ch.PublishWithContext(context.Background(), "", q.Name, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        msg.Body,
		Headers: map[string]interface{}{
			"error": err.Error(),
		},
	})
}

func (p *provider) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.consumerCancel != nil {
		p.consumerCancel()
	}

	if p.ReconnectTicker != nil {
		p.ReconnectTicker.Stop()
	}

	if p.Channel != nil {
		_ = p.Channel.Close()
	}

	if p.Connection != nil {
		return p.Connection.Close()
	}

	return nil
}

func NewClient(rmqpAddress string) (Client, error) {
	client := &provider{
		Consumers:       make(map[string]func([]byte) error),
		ReconnectTicker: time.NewTicker(10 * time.Minute),
	}

	if err := client.Connect(rmqpAddress); err != nil {
		return nil, err
	}

	client.Cache = cache.New(5*time.Minute, 10*time.Minute)

	return client, nil
}
