package server

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func setupTestServer(t *testing.T) (*ssh.Client, func()) {
	// Find a random available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Generate test key
	key, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Create and start SSH server
	server, err := NewSSHServer(port, key)
	if err != nil {
		t.Fatalf("Failed to create SSH server: %v", err)
	}

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Connect client
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("localhost:%d", port), config)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}

	cleanup := func() {
		client.Close()
	}

	return client, cleanup
}

func TestSessionCreation(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer session.Close()
}

func TestCommandExecution(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.Output("echo test")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if string(output) != "test\n" {
		t.Errorf("Expected output 'test\\n', got %q", string(output))
	}
}
