package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ssh2wss/auth"
	"ssh2wss/server"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

// Default ports used by the actual tool
const (
	DefaultSSHPort = 2222
	DefaultWSPort  = 8081
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

func TestBridge(t *testing.T) {
	// Get dynamic ports for testing
	testSSHPort, testWSPort := getFreePorts(t)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create temporary directory for SSH files
	tmpDir, err := os.MkdirTemp("", "ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate test auth token and set it in environment
	testToken := auth.GenerateToken()
	os.Setenv("WSS_AUTH_TOKEN", testToken)
	defer os.Unsetenv("WSS_AUTH_TOKEN")

	// Set up test environment
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")
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

	// Start the bridge with dynamic ports
	bridge, err := server.NewBridge(testSSHPort, testWSPort, hostKey)
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

	// Give the bridge time to start
	time.Sleep(100 * time.Millisecond)

	// Test WebSocket authentication
	t.Run("WebSocket Authentication", func(t *testing.T) {
		tests := []struct {
			name          string
			token         string
			expectSuccess bool
		}{
			{"Valid Token", testToken, true},
			{"Invalid Token", "invalid-token", false},
			{"No Token", "", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create WebSocket config with auth header if token is provided
				config, err := websocket.NewConfig(fmt.Sprintf("ws://localhost:%d/", testWSPort), "http://localhost/")
				if err != nil {
					t.Fatalf("Failed to create WebSocket config: %v", err)
				}

				if tt.token != "" {
					config.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tt.token))
				}

				// Connect to WebSocket server
				ws, err := websocket.DialConfig(config)
				if tt.expectSuccess {
					if err != nil {
						t.Errorf("Expected successful connection, got error: %v", err)
						return
					}
					defer ws.Close()

					// Test echo command
					testMsg := "test message"
					if _, err := ws.Write([]byte("echo " + testMsg)); err != nil {
						t.Errorf("Failed to write to WebSocket: %v", err)
						return
					}

					var response = make([]byte, 1024)
					n, err := ws.Read(response)
					if err != nil {
						t.Errorf("Failed to read from WebSocket: %v", err)
						return
					}

					if !strings.Contains(string(response[:n]), testMsg) {
						t.Errorf("Expected response to contain %q, got %q", testMsg, string(response[:n]))
					}
				} else {
					if err == nil {
						ws.Close()
						t.Error("Expected connection to fail, but it succeeded")
					}
				}
			})
		}
	})

	// Test SSH functionality
	t.Run("Command Execution", func(t *testing.T) {
		tests := []struct {
			name     string
			command  string
			expected string
		}{
			{"ls", "ls -la", "total"},
			{"whoami", "whoami", os.Getenv("USER")},
			{"pwd", "pwd", os.Getenv("PWD")},
			{"echo", "echo 'test message'", "test message"},
			{"env", "env", "HOME="},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sshCmd := exec.CommandContext(ctx, "ssh",
					"-p", fmt.Sprintf("%d", testSSHPort),
					"-o", "StrictHostKeyChecking=no",
					"-o", "UserKnownHostsFile="+knownHostsFile,
					"-o", "LogLevel=ERROR",
					"localhost",
					tt.command)

				output, err := sshCmd.CombinedOutput()
				if err != nil {
					t.Fatalf("SSH command failed: %v\nOutput: %s", err, output)
				}

				if !strings.Contains(string(output), tt.expected) {
					t.Errorf("Expected output to contain %q, got %q", tt.expected, string(output))
				}
			})
		}
	})

	// Test interactive shell
	t.Run("Interactive Shell", func(t *testing.T) {
		// Start SSH in interactive mode
		sshCmd := exec.CommandContext(ctx, "ssh",
			"-tt",
			"-p", fmt.Sprintf("%d", testSSHPort),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+knownHostsFile,
			"-o", "LogLevel=ERROR",
			"localhost")

		stdin, err := sshCmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to get stdin pipe: %v", err)
		}

		var stdout bytes.Buffer
		sshCmd.Stdout = &stdout
		sshCmd.Stderr = &stdout

		if err := sshCmd.Start(); err != nil {
			t.Fatalf("Failed to start SSH: %v", err)
		}

		// Send some commands
		commands := []string{
			"echo $SHELL\n",
			"pwd\n",
			"exit\n",
		}

		for _, cmd := range commands {
			if _, err := io.WriteString(stdin, cmd); err != nil {
				t.Fatalf("Failed to write command: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
		}

		if err := sshCmd.Wait(); err != nil {
			t.Fatalf("SSH session failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "/bin/") {
			t.Errorf("Expected output to contain shell path, got %q", output)
		}
	})
}

func TestGenerateHostKey(t *testing.T) {
	// Test that the same address generates the same key
	addr1 := "localhost:8081"
	key1, err := auth.GenerateKey(addr1)
	if err != nil {
		t.Fatalf("Failed to generate first key: %v", err)
	}

	key2, err := auth.GenerateKey(addr1)
	if err != nil {
		t.Fatalf("Failed to generate second key: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("Keys generated for the same address are different")
	}

	// Test that different addresses generate different keys
	addr2 := "localhost:8082"
	key3, err := auth.GenerateKey(addr2)
	if err != nil {
		t.Fatalf("Failed to generate key for different address: %v", err)
	}

	if bytes.Equal(key1, key3) {
		t.Error("Keys generated for different addresses are the same")
	}

	// Test that the generated key is valid SSH private key
	_, err = ssh.ParsePrivateKey(key1)
	if err != nil {
		t.Errorf("Generated key is not a valid SSH private key: %v", err)
	}
}
