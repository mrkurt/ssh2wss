package commands

import (
	"os"
	"time"
)

func DevCommand(args []string) error {
	// Start server in dev mode in a goroutine
	go func() {
		serverArgs := []string{"-dev"}
		if err := ServerCommand(serverArgs); err != nil {
			os.Exit(1)
		}
	}()

	// Wait a moment for server to start
	time.Sleep(time.Second)

	// Set up environment for proxy
	wsServer := "ws://localhost:8081"
	authToken := os.Getenv("WSS_AUTH_TOKEN")
	os.Setenv("FLYSSH_SERVER", wsServer)
	os.Setenv("FLYSSH_AUTH_TOKEN", authToken)

	// Start proxy
	return ProxyCommand(nil)
}
