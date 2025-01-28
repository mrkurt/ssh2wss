package commands

import (
	"flag"
	"fmt"
	"os"

	"flyssh/internal/client"
)

func ExecCommand(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	command := fs.String("c", "", "Command to execute")
	fs.Parse(args)

	if *command == "" && fs.NArg() > 0 {
		*command = fs.Arg(0)
	}

	if *command == "" {
		return fmt.Errorf("Usage: flyssh [-c] <command>\n   or: flyssh server [options]\n   or: flyssh proxy [options]\n   or: flyssh dev    # Run both server and proxy in development mode")
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
		return fmt.Errorf("failed to create client: %v", err)
	}

	return c.Run()
}
