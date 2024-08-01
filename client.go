package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type provider struct {
	Address string

	Connection *amqp.Connection
	Channel    *amqp.Channel

	Consumers map[string]func([]byte) error
}

type Client interface {
	Run()
	Close() (err error)

	Consume(queueName string, consumer func([]byte) error)
	Send(messageData []byte, queueName string) (err error)
}

type Connection interface {
	Close() error
}

func (p *provider) NewChannel() (err error) {
	p.Channel, err = p.Connection.Channel()

	if err != nil {
		return err
	}

	//go p.AutoConnect(p.Channel.NotifyClose(make(chan *amqp.Error)), p.Channel)

	return nil
}

func (p *provider) Connect(rmqpAddress string) error {
	conn, err := amqp.Dial(rmqpAddress)
	if err != nil {
		return err
	}

	p.Address = rmqpAddress
	p.Connection = conn
	conn.Config.Heartbeat = 15 * time.Minute

	go p.AutoConnect(p.Connection.NotifyClose(make(chan *amqp.Error)), p.Connection)

	return nil
}

func (p *provider) AutoConnect(notifyClose chan *amqp.Error, object Connection) {
	var err error
	defer object.Close()

	for {
		select {
		case reason, ok := <-notifyClose:
			if !ok {
				fmt.Println("Connection closed by the developer")
				return
			}

			fmt.Println("RabbitMQ connection closed", reason.Reason)

			// Attempt to reconnect
			for {
				select {
				case <-context.Background().Done():
					fmt.Println("Context canceled, stopping reconnect attempts")
					return
				default:
					fmt.Println("Attempting to reconnect to RabbitMQ")

					err = p.Connect(p.Address)

					err = p.NewChannel()
					if err == nil {
						fmt.Println("Reconnected successfully to RabbitMQ")
						p.Run()

						return
					}

					fmt.Println("Reconnect failed, retrying")
					time.Sleep(time.Second * 3)
				}
			}

		case <-context.Background().Done():
			fmt.Println("Context canceled, stopping reconnect attempts")
			return
		}
	}
}

func NewClient(rmqpAddress string) (Client, error) {
	client := &provider{
		Consumers: make(map[string]func([]byte) error, 0),
	}
	var err error

	err = client.Connect(rmqpAddress)

	if err != nil {
		return nil, err
	}

	err = client.NewChannel()

	if err != nil {
		return nil, err
	}

	return client, nil
}

func (p *provider) Consume(queueName string, consumer func([]byte) error) {
	p.Consumers[queueName] = consumer
}

func (p *provider) Run() {

	for queueName, consumerFunc := range p.Consumers {
		q, err := p.Channel.QueueDeclare(
			queueName, // name
			false,     // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			nil,       // arguments
		)
		if err != nil {
			log.Fatal("Failed to declare a queue")
		}

		msgs, err := p.Channel.Consume(
			q.Name, // queue
			"",     // consumer
			false,  // auto-ack (set to false for manual acknowledgment)
			false,  // exclusive
			false,  // no-local
			false,  // no-wait
			nil,    // args
		)
		if err != nil {
			log.Fatal("Failed to register a consumer")
		}

		fmt.Println("waiting for", queueName)

		go func(queueName string, consumerFunc func([]byte) error) {
			for msg := range msgs {
				if err := processMessage(p.Channel, msg, consumerFunc); err != nil {
					if err := republishMessage(p.Channel, msg, queueName); err != nil {
						log.Printf("Failed to republish message from queue %s", queueName)
					}
				}
			}
		}(queueName, consumerFunc)
	}

	// Keep the application running to listen for messages
	select {}
}

func processMessage(ch *amqp.Channel, msg amqp.Delivery, consumerFunc func([]byte) error) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in processMessage: %v", r)
		}
	}()

	err := consumerFunc(msg.Body)

	if err != nil {
		return err
	} else {
		return ch.Ack(msg.DeliveryTag, false)
	}
}

func republishMessage(ch *amqp.Channel, msg amqp.Delivery, queueName string) error {

	return ch.PublishWithContext(context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msg.Body,
		})
}

func (p *provider) Send(messageData []byte, queueName string) (err error) {

	q, err := p.Channel.QueueDeclare(
		queueName, // name
		false,     // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)

	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = p.Channel.PublishWithContext(ctx,
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        messageData,
		})

	if err != nil {
		return
	}

	return

}

func (p *provider) Close() (err error) {
	err = p.Channel.Close()

	if err != nil {
		return
	}

	err = p.Connection.Close()

	if err != nil {
		return
	}

	return
}
