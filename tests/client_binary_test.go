package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestClientBinary(t *testing.T) {
	// Start test server
	srv := NewTestServer(t)
	defer srv.Cleanup(t)

	// Create PTY test instance
	pty := NewPTYTest(t)
	defer pty.Cleanup(t)

	// Start monitoring for output
	pty.MonitorOutput(t, "hello")

	// Run client binary command
	clientCmd := fmt.Sprintf("%s client -url %s\n", ClientBinaryPath, srv.URL())
	t.Logf("Running client command: %s", clientCmd)
	if _, err := pty.PTY.Write([]byte(clientCmd)); err != nil {
		t.Fatalf("Failed to write command: %v", err)
	}

	// Wait for client to connect
	time.Sleep(500 * time.Millisecond)

	// Send test command
	t.Log("Sending test command")
	if _, err := pty.PTY.Write([]byte("echo hello\n")); err != nil {
		t.Fatalf("Failed to write test command: %v", err)
	}

	// Wait for output
	if err := pty.WaitForOutput(t, "hello", 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestClientBinaryWithShell(t *testing.T) {
	// Start test server
	srv := NewTestServer(t)
	defer srv.Cleanup(t)

	// Create PTY test instance
	pty := NewPTYTest(t)
	defer pty.Cleanup(t)

	// Start a clean shell
	cmd := exec.Command("/bin/sh", "-i")
	cmd.Env = []string{
		"SHELL=/bin/sh",
		"TERM=dumb",
		"PS1=$ ",
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"WSS_AUTH_TOKEN=" + srv.AuthToken,
	}

	// Start shell with PTY
	if err := pty.StartCommand(t, cmd); err != nil {
		t.Fatal(err)
	}

	// Start monitoring for initial prompt
	pty.MonitorOutput(t, "$")

	// Wait for prompt
	if err := pty.WaitForOutput(t, "$", 2*time.Second); err != nil {
		t.Fatal(err)
	}

	// Reset monitoring for next output
	pty.Done = make(chan bool)
	pty.Output.Reset()

	// Run client binary command
	clientCmd := fmt.Sprintf("%s client -url %s\n", ClientBinaryPath, srv.URL())
	t.Logf("Running client command: %s", clientCmd)
	if _, err := pty.PTY.Write([]byte(clientCmd)); err != nil {
		t.Fatalf("Failed to write command: %v", err)
	}

	// Wait for client to connect
	time.Sleep(1 * time.Second)

	// Reset monitoring for test output
	pty.Done = make(chan bool)
	pty.Output.Reset()
	pty.MonitorOutput(t, "hello")

	// Send test command
	t.Log("Sending test command: echo hello")
	if _, err := pty.PTY.Write([]byte("echo hello\n")); err != nil {
		t.Fatalf("Failed to write test command: %v", err)
	}

	// Wait for output
	if err := pty.WaitForOutput(t, "hello", 5*time.Second); err != nil {
		t.Fatal(err)
	}
}
