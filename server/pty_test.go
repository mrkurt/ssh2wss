package server

import (
	"io"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestPTYRequest(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

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

	// Test echo
	testStr := "test\n"
	if _, err := stdin.Write([]byte(testStr)); err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}

	// Read response
	buf := make([]byte, len(testStr)+1) // +1 for \r
	n, err := stdout.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read from stdout: %v", err)
	}

	if string(buf[:n]) != "test\r\n" {
		t.Errorf("Expected echo %q, got %q", "test\r\n", string(buf[:n]))
	}
}

func TestPTYWindowChange(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

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

	// Test window change
	if err := session.WindowChange(100, 50); err != nil {
		t.Fatalf("Failed to change window size: %v", err)
	}
}
