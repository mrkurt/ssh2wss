package main

import (
	"log"
	"os"
	"strconv"

	"ssh2wss/server"
)

// getEnvInt gets an integer from environment variable or returns default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func main() {
	// Get port numbers from environment or use defaults
	sshPort := getEnvInt("SSH_PORT", server.DefaultSSHPort)
	wsPort := getEnvInt("WS_PORT", server.DefaultWSPort)

	// Create and start bridge
	bridge, err := server.NewBridge(sshPort, wsPort)
	if err != nil {
		log.Fatalf("Failed to create bridge: %v", err)
	}

	// Start bridge
	if err := bridge.Start(); err != nil {
		log.Fatalf("Bridge failed: %v", err)
	}
}
