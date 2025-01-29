package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/client"

	"golang.org/x/crypto/ssh"
)

func ProxyCommand(args []string) error {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	fs.Parse(args)

	wsServer := os.Getenv("FLYSSH_SERVER")
	authToken := os.Getenv("FLYSSH_AUTH_TOKEN")

	// Create SSH client config
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		Auth:            []ssh.AuthMethod{ssh.Password(authToken)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Connect to server
	serverAddr := cleanWSURL(wsServer)
	c, err := client.Connect(serverAddr, config)
	if err != nil {
		return fmt.Errorf("failed to create proxy client: %v", err)
	}
	defer c.Close()

	// Create and start session in proxy mode
	session, err := c.NewInteractiveSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	return session.Start()
}
