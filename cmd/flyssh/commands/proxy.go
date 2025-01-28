package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/internal/client"
)

func ProxyCommand(args []string) error {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	fs.Parse(args)

	wsServer := os.Getenv("FLYSSH_SERVER")
	authToken := os.Getenv("FLYSSH_AUTH_TOKEN")

	config := &client.ClientConfig{
		WSServer:  wsServer,
		AuthToken: authToken,
		IsProxy:   true,
	}

	c, err := client.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create proxy client: %v", err)
	}

	return c.Run()
}
