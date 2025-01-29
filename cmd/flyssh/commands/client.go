package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/core"
)

func ClientCommand(args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)

	// Command line flags
	url := fs.String("url", os.Getenv("WSS_URL"), "WebSocket server URL")
	token := fs.String("token", os.Getenv("WSS_AUTH_TOKEN"), "Auth token")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		return err
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
