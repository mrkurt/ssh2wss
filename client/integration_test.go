package client

import (
	"fmt"
	"os"
	"testing"
	"time"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"

	"flyssh/server"

	"golang.org/x/crypto/ssh"
)

func generateTestKey() ([]byte, error) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %v", err)
	}

	privateBytes := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(private),
	})

	return privateBytes, nil
}

func TestClientInteractiveMode(t *testing.T) {
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
	server, err := server.NewSSHServer(port, key)
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

	// Create client config
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Connect using our client code
	client, err := Connect(fmt.Sprintf("localhost:%d", port), config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Start interactive session
	session, err := client.NewInteractiveSession()
	if err != nil {
		t.Fatalf("Failed to create interactive session: %v", err)
	}
	defer session.Close()

	// Start the shell
	if err := session.Start(); err != nil {
		t.Fatalf("Failed to start shell: %v", err)
	}

	// Test basic commands
	commands := []string{
		"echo 'test message'\n",
		"exit\n",
	}

	for _, cmd := range commands {
		if _, err := session.Write([]byte(cmd)); err != nil {
			t.Errorf("Failed to write command: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for the session to complete with timeout
	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Session wait failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Session wait timed out")
	}
}
