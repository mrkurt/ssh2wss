package main

import (
	"fmt"
	"os"

	"flyssh/cmd/flyssh/commands"
	wsslog "flyssh/core/log"
)

func main() {
	// Initialize logging
	wsslog.Init()

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  flyssh server [-port PORT] [-dev] [-debug]")
		fmt.Println("  flyssh client [-url WS_URL] [-token TOKEN] [-dev] [-debug]")
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "server":
		err = commands.ServerCommand(os.Args[2:])
	case "client":
		err = commands.ClientCommand(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		wsslog.Info.Fatal(err)
	}
}
