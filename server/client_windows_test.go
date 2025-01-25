//go:build windows
// +build windows

package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

func findWindowsSSHClient(t *testing.T) string {
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

func TestWindowsClientToLinuxServer(t *testing.T) {
	// Find OpenSSH client
	sshPath := findWindowsSSHClient(t)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Generate test key
	keyBytes, err := generateWindowsTestKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Parse the key for server use
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	// Create and start SSH server
	server, err := NewSSHServer(port, signer)
	if err != nil {
		t.Fatalf("Failed to create SSH server: %v", err)
	}

	go func() {
		if err := server.Start(); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create temp dir for known_hosts
	tmpDir, err := os.MkdirTemp("", "ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")

	t.Run("Basic Connection", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, sshPath,
			"-tt",
			"-p", fmt.Sprintf("%d", port),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+knownHostsFile,
			"localhost",
			"echo test")

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("SSH command failed: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "test") {
			t.Errorf("Expected output to contain 'test', got: %q", string(output))
		}
	})

	t.Run("Interactive Shell", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, sshPath,
			"-tt",
			"-p", fmt.Sprintf("%d", port),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+knownHostsFile,
			"localhost")

		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to create stdin pipe: %v", err)
		}

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stdout

		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start SSH: %v", err)
		}

		// Send commands with delay to simulate typing
		commands := []string{
			"echo $SHELL\n", // Test Unix shell detection
			"pwd\n",         // Test working directory
			"ls -la\n",      // Test Unix commands work
			"exit\n",
		}

		for _, cmd := range commands {
			time.Sleep(100 * time.Millisecond)
			if _, err := io.WriteString(stdin, cmd); err != nil {
				t.Fatalf("Failed to write command: %v", err)
			}
		}

		if err := cmd.Wait(); err != nil {
			t.Fatalf("SSH session failed: %v\nOutput: %s", err, stdout.String())
		}

		output := stdout.String()
		// Verify Unix shell is detected
		if !strings.Contains(output, "/bin/") {
			t.Errorf("Expected Unix shell path, got output: %q", output)
		}
	})

	t.Run("Terminal Resize", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, sshPath,
			"-tt",
			"-p", fmt.Sprintf("%d", port),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+knownHostsFile,
			"localhost",
			"stty size")

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("SSH command failed: %v\nOutput: %s", err, output)
		}

		// Output should be in format "rows cols"
		parts := strings.Fields(string(output))
		if len(parts) != 2 {
			t.Fatalf("Expected 'rows cols' output, got: %q", string(output))
		}
	})
}

func generateWindowsTestKey() ([]byte, error) {
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
