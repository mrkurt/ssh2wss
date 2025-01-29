package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var clientBinaryPath string

func TestMain(m *testing.M) {
	// Build the client binary once for all tests
	buildCmd := exec.Command("go", "build", "-o", "flyssh", "../cmd/flyssh")
	if err := buildCmd.Run(); err != nil {
		fmt.Printf("Failed to build client binary: %v\n", err)
		os.Exit(1)
	}

	// Get absolute path to binary
	var err error
	clientBinaryPath, err = filepath.Abs("flyssh")
	if err != nil {
		fmt.Printf("Failed to get client binary path: %v\n", err)
		os.Remove("flyssh")
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	os.Remove("flyssh")
	os.Exit(code)
}

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
	clientCmd := fmt.Sprintf("%s client -url %s\n", clientBinaryPath, srv.URL())
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

	// Start a clean zsh shell
	cmd := exec.Command("/bin/zsh", "-f")
	cmd.Env = []string{
		"SHELL=/bin/zsh",
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
	clientCmd := fmt.Sprintf("%s client -url %s\n", clientBinaryPath, srv.URL())
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
