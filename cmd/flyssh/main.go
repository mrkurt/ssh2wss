package main

import (
	"fmt"
	"log"
	"os"

	"flyssh/cmd/flyssh/commands"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  flyssh server [-port PORT] [-dev]")
		fmt.Println("  flyssh client [-url WS_URL] [-token TOKEN] [-dev]")
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
		log.Fatal(err)
	}
}
