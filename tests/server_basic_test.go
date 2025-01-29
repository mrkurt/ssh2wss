package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestServerConnection(t *testing.T) {
	// Get a free port
	port, err := GetFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	// Start server in PTY
	pty := NewPTYTest(t)
	defer pty.Cleanup(t)

	// Set auth token
	authToken := "test-token"
	cmd := exec.Command(ServerBinaryPath, "server", "-port", fmt.Sprintf("%d", port))
	cmd.Env = append(os.Environ(), "WSS_AUTH_TOKEN="+authToken)

	if err := pty.StartCommand(t, cmd); err != nil {
		t.Fatal(err)
	}

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Test valid connection
	origin := "http://localhost"
	url := fmt.Sprintf("ws://localhost:%d?token=%s", port, authToken)
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	ws.Close()
}
