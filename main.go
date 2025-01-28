package main

import (
	"fmt"
	"os"

	"flyssh/cmd/flyssh/commands"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		// No command specified, use client mode with no args
		return commands.ClientCommand([]string{})
	}

	switch os.Args[1] {
	case "server":
		return commands.ServerCommand(os.Args[2:])
	case "proxy":
		return commands.ProxyCommand(os.Args[2:])
	case "client":
		return commands.ClientCommand(os.Args[2:])
	case "dev":
		return commands.DevCommand(os.Args[2:])
	case "-h", "--help":
		printUsage()
		return nil
	default:
		// Unknown command, treat as client mode with all args
		return commands.ClientCommand(os.Args[1:])
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  flyssh [options]                      Connect to remote host via WebSocket")
	fmt.Println("  flyssh server [options]               Run in server mode")
	fmt.Println("  flyssh proxy [options]                Run in proxy mode")
	fmt.Println("  flyssh dev                           Run both server and proxy in development mode")
	fmt.Println("\nRun 'flyssh -h' for options")
}
