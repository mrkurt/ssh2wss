package commands

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"flyssh/client"
	"flyssh/server"

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
	serverURL := fs.String("s", os.Getenv("FLYSSH_SERVER"), "WebSocket server URL")
	token := fs.String("t", os.Getenv("FLYSSH_AUTH_TOKEN"), "Auth token")
	command := fs.String("f", "", "Command to execute (non-interactive mode)")
	dev := fs.Bool("dev", false, "Development mode (starts in-process server)")
	sshPort := fs.Int("ssh-port", 2222, "SSH port for dev mode")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Create SSH client config
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		Auth:            []ssh.AuthMethod{}, // No auth needed for tests
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	var c *client.Client
	var err error

	if *dev {
		// Generate a test key
		key, err := server.GenerateHostKey()
		if err != nil {
			return fmt.Errorf("failed to generate host key: %w", err)
		}

		// Start in-process server
		srv, err := server.NewSSHServer(*sshPort, key)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		// Start server in background
		go func() {
			if err := srv.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			}
		}()

		// Connect directly to SSH server
		c, err = client.ConnectSSH(fmt.Sprintf("localhost:%d", *sshPort), config)
		if err != nil {
			return fmt.Errorf("failed to connect to dev server: %w", err)
		}
	} else {
		// Normal mode - validate required flags
		if *serverURL == "" {
			return fmt.Errorf("WebSocket server URL is required. Set FLYSSH_SERVER or use -s flag")
		}
		if *token == "" {
			return fmt.Errorf("Auth token is required. Set FLYSSH_AUTH_TOKEN or use -t flag")
		}

		// Create WebSocket client and connect
		wsClient := client.NewWebSocketClient(*serverURL, *token, config)
		c, err = wsClient.Connect()
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
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
