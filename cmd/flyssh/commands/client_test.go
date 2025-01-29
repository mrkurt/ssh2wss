package commands

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"flyssh/auth"
	"flyssh/server"
)

// getFreePorts returns available ports for SSH and WebSocket servers
func getFreePorts(t *testing.T) (sshPort, wsPort int) {
	sshListener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to get free port for SSH: %v", err)
	}
	defer sshListener.Close()

	wsListener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to get free port for WebSocket: %v", err)
	}
	defer wsListener.Close()

	sshPort = sshListener.Addr().(*net.TCPAddr).Port
	wsPort = wsListener.Addr().(*net.TCPAddr).Port
	return sshPort, wsPort
}

// setupTestServer starts a test server and returns a cleanup function
func setupTestServer(t *testing.T) (cleanup func(), wsPort int) {
	// Get dynamic test ports
	sshPort, wsPort := getFreePorts(t)

	// Create temporary directory for SSH files
	tmpDir, err := os.MkdirTemp("", "ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Generate test auth token and set it in environment
	testToken := auth.GenerateToken()
	os.Setenv("WSS_AUTH_TOKEN", testToken)

	// Set up test environment
	testKeyFile := filepath.Join(tmpDir, "test_host.key")

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

	// Create bridge server
	bridge, err := server.NewBridge(sshPort, wsPort, hostKey)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// Start bridge in goroutine
	go func() {
		if err := bridge.Start(); err != nil {
			if ctx.Err() == nil { // Only log if not cancelled
				log.Printf("Bridge failed: %v", err)
			}
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Return cleanup function
	cleanup = func() {
		cancel()
		os.RemoveAll(tmpDir)
		os.Unsetenv("WSS_AUTH_TOKEN")
	}

	return cleanup, wsPort
}

func TestClientCommand(t *testing.T) {
	// Test basic client command flags
	t.Run("Flags", func(t *testing.T) {
		tests := []struct {
			name        string
			args        []string
			wantErr     bool
			errContains string
		}{
			{
				name:        "no server url",
				args:        []string{"-t", "test-token"},
				wantErr:     true,
				errContains: "WebSocket server URL is required",
			},
			{
				name:        "no auth token",
				args:        []string{"-s", "ws://localhost:8081"},
				wantErr:     true,
				errContains: "Auth token is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := ClientCommand(tt.args)
				if (err != nil) != tt.wantErr {
					t.Errorf("ClientCommand() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if tt.wantErr && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ClientCommand() error = %v, want error containing %q", err, tt.errContains)
				}
			})
		}
	})

	// Test command execution
	t.Run("Command_Execution", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping command execution test on Windows")
		}

		cleanup, wsPort := setupTestServer(t)
		defer cleanup()

		// Get the auth token that the server is using
		serverToken := os.Getenv("WSS_AUTH_TOKEN")
		if serverToken == "" {
			t.Fatal("WSS_AUTH_TOKEN not set")
		}

		// Test non-interactive command using WebSocket URL
		args := []string{
			"-s", fmt.Sprintf("ws://localhost:%d", wsPort),
			"-t", serverToken,
			"-f", "echo 'test message'",
		}
		err := ClientCommand(args)
		if err != nil {
			t.Errorf("ClientCommand() failed: %v", err)
		}
	})

	// Test connection establishment
	t.Run("Connection_Test", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping connection test on Windows")
		}

		cleanup, wsPort := setupTestServer(t)
		defer cleanup()

		// Get the auth token that the server is using
		serverToken := os.Getenv("WSS_AUTH_TOKEN")
		if serverToken == "" {
			t.Fatal("WSS_AUTH_TOKEN not set")
		}

		// Test that we can establish a connection
		args := []string{
			"-s", fmt.Sprintf("ws://localhost:%d", wsPort),
			"-t", serverToken,
			"-f", "exit 0", // Just run a simple command that exits immediately
		}
		err := ClientCommand(args)
		if err != nil {
			t.Errorf("ClientCommand() connection test failed: %v", err)
		}
	})
}
