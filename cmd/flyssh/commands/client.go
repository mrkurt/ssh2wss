package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"flyssh/core"
)

func ClientCommand(args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	args = fs.Args()
	if len(args) != 1 {
		return fmt.Errorf("usage: client [url]")
	}

	url := args[0]
	token := os.Getenv("WSS_AUTH_TOKEN")
	if token == "" {
		return fmt.Errorf("WSS_AUTH_TOKEN environment variable not set")
	}

	// Create client
	c := core.NewClient(url, token)

	// Create context that's cancelled on interrupt
	//nosemgrep: rules.semgrep.no-background-context
	// This is the application root (client command), so it's appropriate to create
	// the base context here. This context is cancelled on interrupt signal.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	// Connect to server
	return c.Connect(ctx)
}
