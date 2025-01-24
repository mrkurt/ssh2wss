package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestInteractiveSSH(t *testing.T) {
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
			log.Printf("Server error: %v", err)
		}
	}()
	defer func() {
		// Try to kill any remaining processes
		if p := os.Getenv("SSH_TEST_PID"); p != "" {
			fmt.Printf("Cleaning up PID: %s\n", p)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Configure SSH client
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Connect to SSH server
	client, err := ssh.Dial("tcp", fmt.Sprintf("localhost:%d", port), config)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer client.Close()

	// Create a session
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer session.Close()

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		t.Fatalf("Failed to request PTY: %v", err)
	}

	// Setup pipes for stdin/stdout
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	// Start shell
	if err := session.Shell(); err != nil {
		t.Fatalf("Failed to start shell: %v", err)
	}

	// Create a channel to signal command completion
	done := make(chan struct{})

	// Start a goroutine to read stdout
	var output bytes.Buffer
	go func() {
		io.Copy(&output, stdout)
		close(done)
	}()

	// Send a test command
	if _, err := fmt.Fprintf(stdin, "echo 'test message'\nexit\n"); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Wait for command completion or timeout
	select {
	case <-done:
		// Check output
		if !bytes.Contains(output.Bytes(), []byte("test message")) {
			t.Errorf("Expected output to contain 'test message', got: %s", output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out")
	}
}

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

func TestInteractiveSSHWithSubprocess(t *testing.T) {
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
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create a temporary known_hosts file
	tmpDir, err := os.MkdirTemp("", "ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")

	// Test interactive shell
	t.Run("Interactive Shell", func(t *testing.T) {
		cmd := exec.Command("ssh",
			"-tt", // Force TTY allocation
			"-p", fmt.Sprintf("%d", port),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+knownHostsFile,
			"localhost")

		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to get stdin pipe: %v", err)
		}

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stdout

		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start SSH: %v", err)
		}

		// Wait a bit for the connection to establish
		time.Sleep(500 * time.Millisecond)

		// Send commands with delay to simulate typing
		commands := []string{
			"echo 'test message'\n",
			"exit\n",
		}

		for _, cmd := range commands {
			time.Sleep(100 * time.Millisecond)
			if _, err := io.WriteString(stdin, cmd); err != nil {
				t.Fatalf("Failed to write command: %v", err)
			}
		}

		// Wait for command to finish
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("SSH session failed: %v\nOutput: %s", err, stdout.String())
			}
			t.Logf("SSH session completed successfully\nOutput: %s", stdout.String())
		case <-time.After(2 * time.Second):
			t.Errorf("SSH session hung (timed out after 2s)\nOutput so far: %s", stdout.String())
			cmd.Process.Kill()
		}
	})

	// Test non-interactive command
	t.Run("Non-Interactive Command", func(t *testing.T) {
		cmd := exec.Command("ssh",
			"-p", fmt.Sprintf("%d", port),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+knownHostsFile,
			"-o", "LogLevel=DEBUG",
			"localhost",
			"echo 'test message'")

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("SSH command failed: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "test message") {
			t.Errorf("Expected output to contain 'test message', got: %q", string(output))
		}
	})
}

// pipeConn wraps a pipe to implement net.Conn
type pipeConn struct {
	*os.File
}

func (c *pipeConn) LocalAddr() net.Addr  { return pipeAddr{c.Name()} }
func (c *pipeConn) RemoteAddr() net.Addr { return pipeAddr{c.Name()} }

type pipeAddr struct{ name string }

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return a.name }

func TestWindowResize(t *testing.T) {
	// Generate test key
	key, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Find a random available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create SSH server
	server, err := NewSSHServer(port, key)
	if err != nil {
		t.Fatalf("Failed to create SSH server: %v", err)
	}

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create a temporary known_hosts file
	tmpDir, err := os.MkdirTemp("", "ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")

	// Start an SSH session that runs a command that outputs terminal size
	cmd := exec.Command("ssh",
		"-tt",
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile="+knownHostsFile,
		"-o", "LogLevel=QUIET", // Suppress warnings and connection messages
		"localhost",
		"tput -T xterm-256color lines && tput -T xterm-256color cols") // Use -T flag instead of TERM env var

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run SSH command: %v\nOutput: %s", err, stdout.String())
	}

	// Parse the output to get rows and columns (now on separate lines)
	output := strings.TrimSpace(stdout.String())
	lines := strings.Split(output, "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected two lines of output (rows and cols), got: %q", output)
	}

	rows, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		t.Fatalf("Failed to parse rows: %v", err)
	}
	cols, err := strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		t.Fatalf("Failed to parse cols: %v", err)
	}

	t.Logf("Terminal size: %dx%d", rows, cols)

	// Verify the size matches what we expect
	if rows != 24 || cols != 80 {
		t.Errorf("Unexpected terminal size: got %dx%d, want 24x80", rows, cols)
	}
}
