package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"flyssh/auth"
	"flyssh/internal/client"
	"flyssh/server"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			serverCommand(os.Args[2:])
		case "proxy":
			proxyCommand(os.Args[2:])
		case "dev":
			devCommand(os.Args[2:])
			return
		}
		return
	}

	// Default to exec command
	execCommand(os.Args[1:])
}

func execCommand(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	command := fs.String("c", "", "Command to execute")
	fs.Parse(args)

	if *command == "" && fs.NArg() > 0 {
		*command = fs.Arg(0)
	}

	if *command == "" {
		fmt.Println("Usage: flyssh [-c] <command>")
		fmt.Println("   or: flyssh server [options]")
		fmt.Println("   or: flyssh proxy [options]")
		fmt.Println("   or: flyssh dev    # Run both server and proxy in development mode")
		os.Exit(1)
	}

	wsServer := os.Getenv("FLYSSH_SERVER")
	authToken := os.Getenv("FLYSSH_AUTH_TOKEN")

	config := &client.ClientConfig{
		WSServer:  wsServer,
		AuthToken: authToken,
		Command:   *command,
	}

	c, err := client.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}

	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}

func serverCommand(args []string) {
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
			log.Fatalf("Failed to read host key: %v", err)
		}
	} else {
		// Generate host key based on WebSocket address
		wsAddress := fmt.Sprintf("localhost:%d", *wsPort)
		hostKey, err = auth.GenerateKey(wsAddress)
		if err != nil {
			log.Fatalf("Failed to generate host key: %v", err)
		}
	}

	bridge, err := server.NewBridge(*sshPort, *wsPort, hostKey)
	if err != nil {
		log.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		log.Fatalf("Bridge failed: %v", err)
	}
}

func proxyCommand(args []string) {
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
		log.Fatal(err)
	}

	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}

func devCommand(args []string) {
	// Start server in dev mode in a goroutine
	go func() {
		serverArgs := []string{"-dev"}
		serverCommand(serverArgs)
	}()

	// Wait a moment for server to start
	time.Sleep(time.Second)

	// Set up environment for proxy
	wsServer := "ws://localhost:8081"
	authToken := os.Getenv("WSS_AUTH_TOKEN")
	os.Setenv("FLYSSH_SERVER", wsServer)
	os.Setenv("FLYSSH_AUTH_TOKEN", authToken)

	// Start proxy
	proxyCommand(nil)
}
