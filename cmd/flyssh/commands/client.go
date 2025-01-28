package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/internal/client"
)

func ClientCommand(args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)

	// Command line flags
	server := fs.String("s", os.Getenv("FLYSSH_SERVER"), "WebSocket server URL")
	token := fs.String("t", os.Getenv("FLYSSH_AUTH_TOKEN"), "Auth token")
	command := fs.String("f", "", "Command to execute (non-interactive mode)")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate required flags
	if *server == "" {
		return fmt.Errorf("WebSocket server URL is required. Set FLYSSH_SERVER or use -s flag")
	}
	if *token == "" {
		return fmt.Errorf("Auth token is required. Set FLYSSH_AUTH_TOKEN or use -t flag")
	}

	// Create client config
	config := &client.ClientConfig{
		WSServer:  *server,
		AuthToken: *token,
		Command:   *command,
	}

	// Create and run client
	c, err := client.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return c.Run()
}
