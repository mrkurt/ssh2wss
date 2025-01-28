package main

import (
	"log"
	"os"

	"flyssh/cmd/flyssh/commands"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			if err := commands.ServerCommand(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
		case "proxy":
			if err := commands.ProxyCommand(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
		case "dev":
			if err := commands.DevCommand(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		default:
			// Treat unknown commands as part of the command to execute
			if err := commands.ExecCommand(os.Args[1:]); err != nil {
				log.Fatal(err)
			}
		}
		return
	}

	// Default to exec command with no args
	if err := commands.ExecCommand(nil); err != nil {
		log.Fatal(err)
	}
}
