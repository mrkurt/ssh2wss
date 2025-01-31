//go:build unix
// +build unix

package tests

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

func TestClientBinary(t *testing.T) {
	// Test direct client execution (no local shell)
	srv := NewTestServer(t)
	defer srv.Cleanup(t)

	pty := NewPTYTest(t)
	defer pty.Cleanup(t)

	// Run client binary directly
	clientCmd := fmt.Sprintf("%s client -url %s\n", ClientBinaryPath, srv.URL())
	if _, err := pty.PTY.Write([]byte(clientCmd)); err != nil {
		t.Fatalf("Failed to write command: %v", err)
	}

	// Wait for client to connect
	time.Sleep(500 * time.Millisecond)

	// Send test command and verify output
	if _, err := pty.PTY.Write([]byte("echo hello\n")); err != nil {
		t.Fatalf("Failed to write test command: %v", err)
	}

	// Wait for output
	pty.MonitorOutput(t, "hello")
	if err := pty.WaitForOutput(t, "hello", 5*time.Second); err != nil {
		t.Logf("Full output buffer: %q", pty.Output.String())
		t.Fatal(err)
	}
}

func TestClientBinaryWithShell(t *testing.T) {
	// Test client running inside a local shell
	srv := NewTestServer(t)
	defer srv.Cleanup(t)

	pty := NewPTYTest(t)
	defer pty.Cleanup(t)

	// Start a local shell (using same basic setup as server)
	cmd := exec.Command("/bin/sh")
	cmd.Env = []string{
		"TERM=dumb",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"SHELL=/bin/sh",
		"PS1=$ ",
	}
	if err := pty.StartCommand(t, cmd); err != nil {
		t.Fatal(err)
	}

	// Wait for shell prompt
	pty.MonitorOutput(t, "$")
	if err := pty.WaitForOutput(t, "$", 2*time.Second); err != nil {
		t.Logf("Initial shell output: %q", pty.Output.String())
		t.Fatal(err)
	}

	// Run client inside shell
	clientCmd := fmt.Sprintf("%s client -url %s\n", ClientBinaryPath, srv.URL())
	if _, err := pty.PTY.Write([]byte(clientCmd)); err != nil {
		t.Fatalf("Failed to write command: %v", err)
	}

	// Wait for client to connect
	time.Sleep(500 * time.Millisecond)

	// Send test command and verify output
	pty.Done = make(chan bool)
	pty.Output.Reset()
	pty.MonitorOutput(t, "hello")

	if _, err := pty.PTY.Write([]byte("echo hello\n")); err != nil {
		t.Fatalf("Failed to write test command: %v", err)
	}

	if err := pty.WaitForOutput(t, "hello", 5*time.Second); err != nil {
		t.Logf("Full output buffer: %q", pty.Output.String())
		t.Fatal(err)
	}
}
