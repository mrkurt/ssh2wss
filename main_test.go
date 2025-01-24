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
	"runtime"
	"strings"
	"testing"
	"time"

	"ssh2wss/auth"
	"ssh2wss/server"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

// checkGoroutineLeak helps detect goroutine leaks
func checkGoroutineLeak(t *testing.T) func() {
	initialGoroutines := runtime.NumGoroutine()
	return func() {
		time.Sleep(100 * time.Millisecond) // Give goroutines time to clean up
		finalGoroutines := runtime.NumGoroutine()
		if finalGoroutines > initialGoroutines {
			// Get stack traces of all goroutines
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Errorf("Goroutine leak: had %d, now have %d goroutines\nStack traces:\n%s",
				initialGoroutines, finalGoroutines, string(buf[:n]))
		}
	}
}

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
	defer checkGoroutineLeak(t)()

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
	t.Run("Command_Execution", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			// Just check if SSH is available
			sshPath, err := exec.LookPath("ssh")
			t.Logf("SSH binary location: %s (err: %v)", sshPath, err)
		}

		var tests []struct {
			name     string
			command  string
			expected string
		}

		if runtime.GOOS == "windows" {
			tests = []struct {
				name     string
				command  string
				expected string
			}{
				{"directory listing", "dir", "Directory of"},
				{"username", "echo %USERNAME%", os.Getenv("USERNAME")},
				{"current dir", "cd", os.Getenv("CD")},
				{"echo", "echo test message", "test message"},
			}
		} else {
			tests = []struct {
				name     string
				command  string
				expected string
			}{
				{"directory listing", "ls -la", "total"},
				{"whoami", "whoami", os.Getenv("USER")},
				{"pwd", "pwd", os.Getenv("PWD")},
				{"echo", "echo 'test message'", "test message"},
				{"env", "env", "HOME="},
			}
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Logf("Starting SSH command test: %s", tt.name)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				sshCmd := exec.CommandContext(ctx, "ssh",
					"-p", fmt.Sprintf("%d", testSSHPort),
					"-o", "StrictHostKeyChecking=no",
					"-o", "UserKnownHostsFile="+knownHostsFile,
					"-o", "LogLevel=ERROR",
					"localhost",
					tt.command)

				t.Logf("Running SSH command: %v", sshCmd.Args)
				output, err := sshCmd.CombinedOutput()
				t.Logf("SSH command completed with error: %v", err)
				if err != nil {
					t.Fatalf("SSH command failed: %v\nOutput: %s", err, output)
				}

				t.Logf("Command output: %s", string(output))
				if !strings.Contains(string(output), tt.expected) {
					t.Errorf("Expected output to contain %q, got %q", tt.expected, string(output))
				}
				t.Logf("Test completed successfully")
			})
		}
	})

	// Test interactive shell
	t.Run("Interactive Shell", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("TODO: Implement proper Windows console/PTY support for interactive shells")
		}

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

		// Send commands
		commands := []string{
			"echo $SHELL\n",
			"pwd\n",
			"echo ready\n",
			"exit\n",
		}

		for _, cmd := range commands {
			t.Logf("Sending command: %q", cmd)
			if _, err := io.WriteString(stdin, cmd); err != nil {
				t.Fatalf("Failed to write command: %v", err)
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Wait for command completion
		if err := sshCmd.Wait(); err != nil {
			t.Logf("SSH session ended with error: %v", err)
			t.Logf("Full output:\n%s", stdout.String())
			t.FailNow()
		}

		// Verify we got some output
		output := stdout.String()
		if !strings.Contains(output, "ready") {
			t.Errorf("Expected to see 'ready' in output, got:\n%s", output)
		}
	})

	// Clean up
	bridge.Stop()
	time.Sleep(100 * time.Millisecond) // Give servers time to shut down
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
