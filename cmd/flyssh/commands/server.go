package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/core"
)

func ServerCommand(args []string) error {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:8081", "Listen address (e.g. 127.0.0.1:8081, :2222, 0.0.0.0:2222)")
	devMode := fs.Bool("dev", false, "Run in development mode with auto-generated token")
	fs.Parse(args)

	// In dev mode, generate a token and set it in the environment
	if *devMode {
		token := core.GenerateDevToken()
		os.Setenv("WSS_AUTH_TOKEN", token)
		fmt.Printf("\n=== Development Mode ===\n")
		fmt.Printf("WebSocket URL: ws://%s\n", *listen)
		fmt.Printf("Auth Token: %s\n", token)
		fmt.Printf("====================\n\n")
	}

	// Create and start server
	s := core.NewServer(*listen)
	return s.Start()
}

// generateToken creates a random token for development mode
func generateToken() string {
	const tokenLength = 32
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, tokenLength)
	for i := range result {
		result[i] = chars[i%len(chars)]
	}
	return string(result)
}
