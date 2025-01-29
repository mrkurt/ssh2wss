package commands

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"flyssh/client"

	"golang.org/x/crypto/ssh"
)

// cleanWSURL converts a WebSocket URL to a host:port format
func cleanWSURL(url string) string {
	// Strip ws:// or wss:// prefix
	url = strings.TrimPrefix(url, "ws://")
	url = strings.TrimPrefix(url, "wss://")
	// Remove any path component
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}
	return url
}

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

	// Create SSH client config
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		Auth:            []ssh.AuthMethod{}, // No auth needed for tests
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Create WebSocket client
	wsClient := client.NewWebSocketClient(*server, *token, config)

	// Connect to server
	c, err := wsClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	// Create and start session
	session, err := c.NewInteractiveSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	if *command != "" {
		// Non-interactive mode
		return session.Run(*command)
	}

	// Interactive mode
	return session.Start()
}
