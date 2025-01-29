package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"flyssh/core"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  flyssh server [-port PORT]")
		fmt.Println("  flyssh client -url WS_URL")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		// Parse server flags
		cmd := flag.NewFlagSet("server", flag.ExitOnError)
		port := cmd.Int("port", 8081, "Port to listen on")
		cmd.Parse(os.Args[2:])

		// Start server
		s := core.NewServer(*port)
		log.Printf("Starting WebSocket server on :%d", *port)
		if err := s.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}

	case "client":
		// Parse client flags
		cmd := flag.NewFlagSet("client", flag.ExitOnError)
		url := cmd.String("url", "", "WebSocket URL to connect to (e.g. ws://localhost:8081)")
		cmd.Parse(os.Args[2:])

		if *url == "" {
			fmt.Println("Error: -url is required")
			cmd.PrintDefaults()
			os.Exit(1)
		}

		// Start client
		c := core.NewClient(*url, os.Getenv("WSS_AUTH_TOKEN"))
		if err := c.Connect(); err != nil {
			log.Fatalf("Client error: %v", err)
		}

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
