package commands

import (
	"flag"
	"fmt"
	"math/rand"
	"os"

	"flyssh/core"
)

func ClientCommand(args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)

	// Command line flags
	url := fs.String("url", os.Getenv("WSS_URL"), "WebSocket server URL")
	token := fs.String("token", os.Getenv("WSS_AUTH_TOKEN"), "Auth token")
	dev := fs.Bool("dev", false, "Run in development mode with local server")
	debug := fs.Bool("debug", false, "Enable debug logging")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Enable debug logging if flag is set
	if *debug {
		os.Setenv("WSS_DEBUG", "1")
	}

	// In dev mode, start server in background and set URL/token
	if *dev {
		// Use random high port (49152-65535)
		port := rand.Intn(65535-49152) + 49152
		devToken := core.GenerateDevToken()
		os.Setenv("WSS_AUTH_TOKEN", devToken)
		*url = fmt.Sprintf("ws://localhost:%d", port)
		*token = devToken // Set the token flag value too

		// Start server in background
		s := core.NewServer(port)
		go s.Start()

		fmt.Printf("\n=== Development Mode ===\n")
		fmt.Printf("WebSocket URL: %s\n", *url)
		fmt.Printf("Auth Token: %s\n", *token)
		fmt.Printf("====================\n\n")
	}

	// Validate required flags
	if *url == "" {
		return fmt.Errorf("WebSocket URL is required. Set WSS_URL or use -url flag")
	}
	if *token == "" {
		return fmt.Errorf("Auth token is required. Set WSS_AUTH_TOKEN or use -token flag")
	}

	// Create and start client
	c := core.NewClient(*url, *token)
	return c.Connect()
}
