
# go-rabbitmq-client

**go-rabbitmq-client** is a lightweight Go library for interacting with RabbitMQ. It provides features like auto-reconnect and panic handling, making it a reliable choice for RabbitMQ communication.

## Features

1. **Autoconnect**: The library automatically reconnects to the RabbitMQ server if the connection is lost. This ensures that your application remains resilient even in the face of network interruptions or RabbitMQ server restarts.

2. **Panic Catcher**: go-rabbitmq-client includes a panic catcher mechanism. If an unexpected panic occurs during message processing, the library gracefully handles it, preventing your application from crashing. This is especially useful in long-running services.

3. **Lightweight**: The library is designed to be minimalistic and efficient. It doesn't introduce unnecessary overhead, making it suitable for resource-constrained environments.

## Installation

To use go-rabbitmq-client in your Go project, simply import it:

```go
import "github.com/Jacobamv/go-rabbitmq-client"
```

## Usage

Here's a basic example of how to use go-rabbitmq-client:

```go
package main

import (
	"fmt"
	rabbitmq "github.com/Jacobamv/go-rabbitmq-client"
)

func Test(msg []byte) error {
	fmt.Println("got a message", string(msg))

	return nil
}

func main() {

	rmqp := fmt.Sprintf("amqp://%v:%v@%v:%v/",
		"guest",
		"guest",
		"localhost",
		"5672",
	)

	client, err := rabbitmq.NewClient(rmqp)

	if err != nil {
		return
	}

	client.Consume("test", Test)

	client.Run()
}

```

Remember to adjust the connection URL (`"amqp://guest:guest@localhost:5672/"`) and other settings according to your RabbitMQ setup.

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests on the [GitHub repository](https://github.com/Jacobamv/go-rabbitmq-client).

## License

This project is licensed under the GNU 3 LICENSE. See the [LICENSE](LICENSE) file for details.

---


: [RabbitMQ Official Documentation](https://www.rabbitmq.com/documentation.html)
: [go-rabbitmq-client GitHub Repository](https://github.com/Jacobamv/go-rabbitmq-client)
