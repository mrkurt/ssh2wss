package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"flyssh/client"
	"flyssh/server"
)

func main() {
	// Define command-line flags
	serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
	serverPort := serverCmd.Int("port", 8081, "Port to listen on")

	clientCmd := flag.NewFlagSet("client", flag.ExitOnError)
	clientURL := clientCmd.String("url", "", "WebSocket URL to connect to (e.g. ws://localhost:8081)")

	// Parse command
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  flyssh server [-port PORT]")
		fmt.Println("  flyssh client -url WS_URL")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		serverCmd.Parse(os.Args[2:])
		runServer(*serverPort)
	case "client":
		clientCmd.Parse(os.Args[2:])
		if *clientURL == "" {
			fmt.Println("Error: -url is required")
			clientCmd.PrintDefaults()
			os.Exit(1)
		}
		runClient(*clientURL)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runServer(port int) {
	// Check for auth token
	authToken := os.Getenv("WSS_AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("WSS_AUTH_TOKEN environment variable must be set")
	}

	// Create and start server
	s := server.New(port)
	log.Printf("Starting server on port %d", port)
	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func runClient(url string) {
	// Check for auth token
	authToken := os.Getenv("WSS_AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("WSS_AUTH_TOKEN environment variable must be set")
	}

	// Create and connect client
	c := client.New(url, authToken)
	if err := c.Connect(); err != nil {
		log.Fatalf("Client error: %v", err)
	}
}
