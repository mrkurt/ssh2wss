package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/auth"
	"flyssh/server"
)

func ServerCommand(args []string) error {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	sshPort := fs.Int("ssh-port", 2222, "SSH server port")
	wsPort := fs.Int("ws-port", 8081, "WebSocket server port")
	keyPath := fs.String("key", "", "Optional path to SSH host key (if not provided, will generate based on WebSocket address)")
	devMode := fs.Bool("dev", false, "Run in development mode with auto-generated token")
	fs.Parse(args)

	// In dev mode, generate a token and set it in the environment
	if *devMode {
		token := auth.GenerateToken()
		os.Setenv("WSS_AUTH_TOKEN", token)
		fmt.Printf("\n=== Development Mode ===\n")
		fmt.Printf("WebSocket URL: ws://localhost:%d\n", *wsPort)
		fmt.Printf("Auth Token: %s\n", token)
		fmt.Printf("SSH Command: ssh -p %d localhost\n", *sshPort)
		fmt.Printf("====================\n\n")
	}

	var hostKey []byte
	var err error

	if *keyPath != "" {
		// Use provided host key file
		hostKey, err = os.ReadFile(*keyPath)
		if err != nil {
			return fmt.Errorf("failed to read host key: %v", err)
		}
	} else {
		// Generate host key based on WebSocket address
		wsAddress := fmt.Sprintf("localhost:%d", *wsPort)
		hostKey, err = auth.GenerateKey(wsAddress)
		if err != nil {
			return fmt.Errorf("failed to generate host key: %v", err)
		}
	}

	bridge, err := server.NewBridge(*sshPort, *wsPort, hostKey)
	if err != nil {
		return fmt.Errorf("failed to create bridge: %v", err)
	}

	return bridge.Start()
}
