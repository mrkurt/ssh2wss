package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

const (
	testSSHPort = 2222
	testWSPort  = 8081
)

func TestBridge(t *testing.T) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create temporary directory for SSH files
	tmpDir, err := os.MkdirTemp("", "ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")
	testKeyFile := filepath.Join(tmpDir, "test_host.key")

	// Start WebSocket server
	var serverReady sync.WaitGroup
	serverReady.Add(1)

	mux := http.NewServeMux()
	mux.Handle("/", websocket.Handler(func(ws *websocket.Conn) {
		log.Printf("WebSocket connection established")
		var buf [1024]byte
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := ws.Read(buf[:])
				if err != nil {
					if err != io.EOF {
						log.Printf("WebSocket read error: %v", err)
					}
					return
				}
				cmd := string(buf[:n])
				log.Printf("WebSocket received command: %s", cmd)

				// Parse and execute the command
				if strings.HasPrefix(cmd, "echo") {
					response := strings.TrimPrefix(cmd, "echo ")
					response = strings.TrimSpace(response)
					response = fmt.Sprintf("%s\n", response)
					log.Printf("Sending response: %q", response)
					if _, err := ws.Write([]byte(response)); err != nil {
						log.Printf("WebSocket write error: %v", err)
						return
					}
					// Close the connection after sending the response
					ws.Close()
					return
				}
			}
		}
	}))

	// Create server with proper shutdown
	server := &http.Server{
		Handler: mux,
	}

	// Create listener first to ensure port is available
	listener, err := net.Listen("tcp", ":8081")
	if err != nil {
		t.Fatalf("Failed to listen on port 8081: %v", err)
	}

	// Signal that server is ready to accept connections
	serverReady.Done()
	log.Printf("WebSocket server ready on port %d", testWSPort)

	// Start server in goroutine
	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// Ensure server cleanup
	defer func() {
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}()

	// Generate test SSH key
	cmd := exec.Command("ssh-keygen", "-t", "rsa", "-f", testKeyFile, "-N", "")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to generate test SSH key: %v", err)
	}

	// Read the host key
	hostKey, err := os.ReadFile(testKeyFile)
	if err != nil {
		t.Fatalf("Failed to read host key: %v", err)
	}

	// Start the bridge
	bridge, err := NewBridge(testSSHPort, "ws://localhost:8081", hostKey)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// Start bridge in goroutine with context
	bridgeCtx, bridgeCancel := context.WithCancel(ctx)
	defer bridgeCancel()

	go func() {
		if err := bridge.Start(); err != nil {
			if bridgeCtx.Err() == nil { // Only log if not cancelled
				log.Printf("Bridge failed: %v", err)
			}
		}
	}()

	// Wait for server to be ready with timeout
	done := make(chan struct{})
	go func() {
		serverReady.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("Server ready signal received")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for WebSocket server")
	}

	// Give servers a moment to fully initialize
	time.Sleep(100 * time.Millisecond)

	// Test SSH connection with custom known_hosts file and timeout
	log.Printf("Starting SSH test")
	sshCmd := exec.CommandContext(ctx, "ssh",
		"-p", "2222",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile="+knownHostsFile,
		"-o", "LogLevel=ERROR", // Suppress known hosts warnings
		"localhost",
		"echo test")

	output, err := sshCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("SSH command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if outputStr != "test\n" {
		t.Errorf("Expected output 'test\\n', got %q", outputStr)
	}
	log.Printf("Test completed successfully")
}
