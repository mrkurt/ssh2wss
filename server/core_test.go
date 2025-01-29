package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// generateTestKey generates a test RSA key for the SSH server
func generateTestKey() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return pem.EncodeToMemory(privateKeyPEM), nil
}

func TestServerSetup(t *testing.T) {
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

	// Test logger setup
	customLogger := log.New(os.Stderr, "TEST: ", log.LstdFlags)
	server.SetLogger(customLogger)

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test basic connection
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("localhost:%d", port), config)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer client.Close()
}
