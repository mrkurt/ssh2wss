//go:build windows
// +build windows

package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type testServer struct {
	config *ssh.ServerConfig
	addr   string
	ln     net.Listener
}

func findWindowsOpenSSH(t *testing.T) string {
	// Check System32 first (where Windows OpenSSH is typically installed)
	system32 := os.Getenv("SystemRoot") + "\\System32\\OpenSSH"
	sshPath := filepath.Join(system32, "ssh.exe")
	if _, err := os.Stat(sshPath); err == nil {
		return sshPath
	}

	// Try PATH
	path, err := exec.LookPath("ssh.exe")
	if err == nil {
		return path
	}

	t.Skip("Windows OpenSSH client (ssh.exe) not found")
	return ""
}

func TestWindowsSSHConnection(t *testing.T) {
	sshPath := findWindowsOpenSSH(t)

	// Create test keys
	key := []byte(`
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACB8ZgmRmXruvfVcVarFJKlcWIY6VahMFx2IHDIWblj74QAAAJhF9pdYRfaX
WAAAAAtzc2gtZWQyNTUxOQAAACB8ZgmRmXruvfVcVarFJKlcWIY6VahMFx2IHDIWblj74Q
AAAECxk9LT7TvxXxqwxQWGHJzTn1M1H9VNpLHhY0qL9sBpw3xmCZGZeu699VxVqsUkqVxY
hjpVqEwXHYgcMhZuWPvhAAAADnRlc3RAc29tZXdoZXJlAQIDBAUGBw==
-----END OPENSSH PRIVATE KEY-----
`)
	hostKey, err := ssh.ParsePrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to parse host key: %v", err)
	}

	// Configure the server
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(hostKey)

	// Start listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}

	server := &testServer{
		config: config,
		addr:   ln.Addr().String(),
		ln:     ln,
	}

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.serve()
	}()

	// Get the port
	_, port, err := net.SplitHostPort(server.addr)
	if err != nil {
		t.Fatalf("Failed to get server port: %v", err)
	}

	t.Run("Interactive Shell", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Connect using Windows OpenSSH
		cmd := exec.CommandContext(ctx, sshPath,
			"-tt", // Force PTY allocation
			"-p", port,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"test@127.0.0.1")

		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to create stdin pipe: %v", err)
		}

		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start SSH: %v", err)
		}

		// Send commands
		commands := []string{
			"echo %USERNAME%",
			"set TEST_VAR=hello",
			"echo %TEST_VAR%",
			"exit",
		}

		for _, command := range commands {
			fmt.Fprintf(stdin, "%s\r\n", command)
		}

		if err := cmd.Wait(); err != nil {
			t.Fatalf("SSH session failed: %v\nOutput: %s", err, output.String())
		}

		// Verify output
		outputStr := output.String()
		if !strings.Contains(outputStr, os.Getenv("USERNAME")) {
			t.Errorf("Expected output to contain username, got: %q", outputStr)
		}
		if !strings.Contains(outputStr, "hello") {
			t.Errorf("Expected output to contain 'hello', got: %q", outputStr)
		}
	})

	t.Run("Interactive Command", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Use 'type con' as Windows equivalent of 'cat'
		cmd := exec.CommandContext(ctx, sshPath,
			"-tt", // Force PTY allocation
			"-p", port,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"test@127.0.0.1")

		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to create stdin pipe: %v", err)
		}

		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start SSH: %v", err)
		}

		// Send test input
		testInput := "Hello from Windows\r\n"
		fmt.Fprintf(stdin, "type con\r\n")
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(stdin, testInput)
		fmt.Fprintf(stdin, "\x1A") // Ctrl+Z to end input
		fmt.Fprintf(stdin, "\r\nexit\r\n")

		if err := cmd.Wait(); err != nil {
			t.Fatalf("SSH session failed: %v\nOutput: %s", err, output.String())
		}

		// Verify output contains our test input
		if !strings.Contains(output.String(), "Hello from Windows") {
			t.Errorf("Expected output to contain test input, got: %q", output.String())
		}
	})

	t.Run("Subprocess", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, sshPath,
			"-tt", // Force PTY allocation
			"-p", port,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"test@127.0.0.1")

		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to create stdin pipe: %v", err)
		}

		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start SSH: %v", err)
		}

		// Start a subprocess and interact with it
		commands := []string{
			"powershell -NoProfile -NonInteractive",
			"$Host.UI.RawUI.WindowTitle = 'Test Window'",
			"Write-Host $env:USERNAME",
			"exit",
			"echo Returned to cmd",
			"exit",
		}

		for _, command := range commands {
			fmt.Fprintf(stdin, "%s\r\n", command)
		}

		if err := cmd.Wait(); err != nil {
			t.Fatalf("SSH session failed: %v\nOutput: %s", err, output.String())
		}

		outputStr := output.String()
		if !strings.Contains(outputStr, os.Getenv("USERNAME")) {
			t.Errorf("Expected output to contain username, got: %q", outputStr)
		}
		if !strings.Contains(outputStr, "Returned to cmd") {
			t.Errorf("Expected output to show return to cmd, got: %q", outputStr)
		}
	})

	t.Run("Window Resize", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// First check with default size
		cmd := exec.CommandContext(ctx, sshPath,
			"-tt", // Force PTY allocation
			"-p", port,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"test@127.0.0.1",
			"mode con")

		output1, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("SSH command failed: %v\nOutput: %s", err, output1)
		}

		// Then check with custom size
		cmd = exec.CommandContext(ctx, sshPath,
			"-tt", // Force PTY allocation
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-p", port,
			"test@127.0.0.1",
			"powershell -NoProfile -NonInteractive -Command \"$Host.UI.RawUI.WindowSize = New-Object System.Management.Automation.Host.Size(120, 40); mode con\"")

		output2, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("SSH command failed: %v\nOutput: %s", err, output2)
		}

		// Verify both outputs contain console dimensions
		if !bytes.Contains(output1, []byte("Lines:")) || !bytes.Contains(output1, []byte("Columns:")) {
			t.Errorf("Expected console dimensions in first check, got: %q", output1)
		}
		if !bytes.Contains(output2, []byte("Lines:")) || !bytes.Contains(output2, []byte("Columns:")) {
			t.Errorf("Expected console dimensions in second check, got: %q", output2)
		}

		// Verify dimensions changed
		if bytes.Equal(output1, output2) {
			t.Error("Expected different console dimensions after resize")
		}
	})

	// Shutdown server
	ln.Close()

	// Check server error
	if err := <-errCh; err != nil && !isClosedError(err) {
		t.Fatalf("Server error: %v", err)
	}
}

func (s *testServer) serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return err
		}

		go s.handleConn(conn)
	}
}

func (s *testServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}
	defer sshConn.Close()

	// Service incoming requests
	go ssh.DiscardRequests(reqs)

	// Service the incoming channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}

		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "shell", "exec", "pty-req", "window-change":
					req.Reply(true, nil)
				default:
					req.Reply(false, nil)
				}
			}
		}(requests)

		go io.Copy(channel, channel)
	}
}

func isClosedError(err error) bool {
	return err.Error() == "accept tcp 127.0.0.1:0: use of closed network connection"
}
