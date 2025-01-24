package main

import (
	"bufio"
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
	"ssh2wss/auth"
	"ssh2wss/server"
	"strings"
	"testing"
	"time"

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

	// Start the bridge with dynamic ports
	bridge, err := server.NewBridge(testSSHPort, testWSPort)
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
				sshCmd := exec.Command("ssh",
					"-p", fmt.Sprintf("%d", testSSHPort),
					"-o", "StrictHostKeyChecking=no",
					"-o", "UserKnownHostsFile="+knownHostsFile,
					"-o", "LogLevel=ERROR",
					"localhost",
					tt.command)

				t.Logf("Running SSH command: %v", sshCmd.Args)

				// Set up pipes for stdout and stderr
				stdout, err := sshCmd.StdoutPipe()
				if err != nil {
					t.Fatalf("Failed to create stdout pipe: %v", err)
				}
				stderr, err := sshCmd.StderrPipe()
				if err != nil {
					t.Fatalf("Failed to create stderr pipe: %v", err)
				}

				// Start the command
				if err := sshCmd.Start(); err != nil {
					t.Fatalf("Failed to start SSH command: %v", err)
				}

				// Create a channel to signal when output processing is done
				outputDone := make(chan struct{})

				// Process output in a goroutine
				var output bytes.Buffer
				go func() {
					defer close(outputDone)

					// Copy stdout and stderr to both t.Log and our output buffer
					go func() {
						scanner := bufio.NewScanner(stdout)
						for scanner.Scan() {
							line := scanner.Text()
							t.Logf("stdout: %s", line)
							output.WriteString(line + "\n")
						}
					}()

					go func() {
						scanner := bufio.NewScanner(stderr)
						for scanner.Scan() {
							line := scanner.Text()
							t.Logf("stderr: %s", line)
							output.WriteString(line + "\n")
						}
					}()
				}()

				// Wait for command to complete
				err = sshCmd.Wait()
				t.Logf("SSH command completed with error: %v", err)
				if err != nil {
					t.Fatalf("SSH command failed: %v", err)
				}

				// Wait for output processing to complete
				<-outputDone

				// Check output
				outputStr := output.String()
				t.Logf("Command output: %s", outputStr)
				if !strings.Contains(outputStr, tt.expected) {
					t.Errorf("Expected output to contain %q, got %q", tt.expected, outputStr)
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

	t.Run("WebSocket_Fragmented", func(t *testing.T) {
		// Create a TCP connection directly
		conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", testWSPort))
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		// Send HTTP WebSocket upgrade request
		req := fmt.Sprintf("GET /ssh HTTP/1.1\r\n"+
			"Host: localhost:%d\r\n"+
			"Authorization: Bearer %s\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: websocket\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
			testWSPort, testToken)

		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("Failed to write upgrade request: %v", err)
		}

		// Read upgrade response
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}
			if line == "\r\n" {
				break // End of headers
			}
		}

		// Send command one byte at a time
		command := "echo fragmented message"
		for i := 0; i < len(command); i++ {
			_, err = conn.Write([]byte{command[i]})
			if err != nil {
				t.Fatalf("Failed to write byte: %v", err)
			}
			time.Sleep(10 * time.Millisecond) // Small delay between bytes
		}

		// Read response
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}
		t.Logf("Response: %s", string(buf[:n]))
	})

	t.Run("WebSocket_Edge_Cases", func(t *testing.T) {
		tests := []struct {
			name                string
			send                func(net.Conn) error
			expectProtocolError bool
		}{
			{
				name: "Invalid Frame Header",
				send: func(conn net.Conn) error {
					// Send invalid WebSocket frame header
					_, err := conn.Write([]byte{0x82, 0x00}) // Invalid opcode
					return err
				},
				expectProtocolError: true,
			},
			{
				name: "Fragmented Text Frame",
				send: func(conn net.Conn) error {
					// Send fragmented text frame without FIN bit
					_, err := conn.Write([]byte{0x01, 0x03, 'e', 'c', 'h'})
					return err
				},
				expectProtocolError: true,
			},
			{
				name: "Oversized Frame",
				send: func(conn net.Conn) error {
					// Send frame with invalid length
					_, err := conn.Write([]byte{0x81, 0xFF, 0xFF, 0xFF, 0xFF})
					return err
				},
				expectProtocolError: true,
			},
			{
				name: "Invalid UTF-8",
				send: func(conn net.Conn) error {
					// Send invalid UTF-8 sequence in text frame
					_, err := conn.Write([]byte{0x81, 0x02, 0xFF, 0xFF})
					return err
				},
				expectProtocolError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create a TCP connection directly
				conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", testWSPort))
				if err != nil {
					t.Fatalf("Failed to connect: %v", err)
				}
				defer conn.Close()

				// Send HTTP WebSocket upgrade request
				req := fmt.Sprintf("GET /ssh HTTP/1.1\r\n"+
					"Host: localhost:%d\r\n"+
					"Authorization: Bearer %s\r\n"+
					"Connection: Upgrade\r\n"+
					"Upgrade: websocket\r\n"+
					"Sec-WebSocket-Version: 13\r\n"+
					"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
					testWSPort, testToken)

				if _, err := conn.Write([]byte(req)); err != nil {
					t.Fatalf("Failed to write upgrade request: %v", err)
				}

				// Read upgrade response
				reader := bufio.NewReader(conn)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						t.Fatalf("Failed to read response: %v", err)
					}
					if line == "\r\n" {
						break // End of headers
					}
				}

				// Send test-specific data
				if err := tt.send(conn); err != nil {
					t.Fatalf("Failed to send data: %v", err)
				}

				// Read response - should get EOF or protocol error
				buf := make([]byte, 1024)
				_, err = conn.Read(buf)
				if err == nil {
					t.Error("Expected connection to be closed after protocol error")
				}
			})
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
