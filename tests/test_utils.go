package tests

import (
	"flyssh/core"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// ClientBinaryPath is the path to the client binary
var ClientBinaryPath string

// ServerBinaryPath is the path to the server binary
var ServerBinaryPath string

func init() {
	// Create tmp directory if it doesn't exist
	if err := os.MkdirAll("tmp", 0755); err != nil {
		panic(fmt.Sprintf("Failed to create tmp directory: %v", err))
	}

	// Build the client binary
	buildCmd := exec.Command("go", "build", "-o", "tmp/flyssh", "../cmd/flyssh")
	if err := buildCmd.Run(); err != nil {
		os.RemoveAll("tmp")
		panic(fmt.Sprintf("Failed to build client binary: %v", err))
	}

	// Get absolute path to binaries
	var err error
	ClientBinaryPath, err = filepath.Abs("tmp/flyssh")
	if err != nil {
		os.RemoveAll("tmp")
		panic(fmt.Sprintf("Failed to get client binary path: %v", err))
	}
	ServerBinaryPath = ClientBinaryPath // Same binary, different command
}

// TestServer represents a test server instance
type TestServer struct {
	Server     *core.Server
	Port       int
	Done       chan error
	AuthToken  string
	WorkingDir string
	Debug      bool
}

// NewTestServer creates and starts a test server
func NewTestServer(t *testing.T) *TestServer {
	port, err := GetFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	// Set auth token
	authToken := "test-token"
	os.Setenv("WSS_AUTH_TOKEN", authToken)

	// Start server
	srv := core.NewServer(port)
	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	return &TestServer{
		Server:    srv,
		Port:      port,
		Done:      done,
		AuthToken: authToken,
		Debug:     os.Getenv("TEST_DEBUG") == "1",
	}
}

// Cleanup stops the server and cleans up environment
func (ts *TestServer) Cleanup(t *testing.T) {
	t.Log("Stopping server")
	ts.Server.Stop()
	os.Unsetenv("WSS_AUTH_TOKEN")
}

// URL returns the WebSocket URL for the test server
func (ts *TestServer) URL() string {
	return fmt.Sprintf("ws://localhost:%d", ts.Port)
}

// PTYTest represents a PTY test instance
type PTYTest struct {
	PTY    *os.File
	TTY    *os.File
	Output strings.Builder
	Done   chan bool
	Cmd    *exec.Cmd
	Debug  bool
}

// NewPTYTest creates a new PTY test instance
func NewPTYTest(t *testing.T) *PTYTest {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("Failed to open PTY: %v", err)
	}

	// Set initial window size
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: 24,
		Cols: 80,
	}); err != nil {
		t.Fatalf("Failed to set window size: %v", err)
	}

	return &PTYTest{
		PTY:   ptmx,
		TTY:   tty,
		Done:  make(chan bool),
		Debug: os.Getenv("TEST_DEBUG") == "1",
	}
}

// MonitorOutput starts monitoring PTY output for a specific string
func (pt *PTYTest) MonitorOutput(t *testing.T, target string) {
	go func() {
		defer close(pt.Done)
		buf := make([]byte, 32*1024)
		for {
			n, err := pt.PTY.Read(buf)
			if err != nil {
				if err != io.EOF {
					t.Errorf("Failed to read output: %v", err)
				}
				return
			}
			if pt.Debug {
				t.Logf("Read from PTY: %q", buf[:n])
			}
			pt.Output.Write(buf[:n])
			if strings.Contains(pt.Output.String(), target) {
				if pt.Debug {
					t.Logf("Found expected output")
				}
				return
			}
		}
	}()
}

// WaitForOutput waits for the target string in output or times out
func (pt *PTYTest) WaitForOutput(t *testing.T, target string, timeout time.Duration) error {
	select {
	case <-pt.Done:
		if !strings.Contains(pt.Output.String(), target) {
			return fmt.Errorf("expected output to contain %q, got: %q", target, pt.Output.String())
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for output. Got so far: %q", pt.Output.String())
	}
}

// Cleanup sends Ctrl+C and closes the PTY
func (pt *PTYTest) Cleanup(t *testing.T) {
	if pt.Debug {
		t.Log("Starting PTY cleanup...")
	}

	// Send Ctrl+C (ASCII ETX - End of Text, code 3)
	if pt.Debug {
		t.Log("Sending Ctrl+C")
	}
	if _, err := pt.PTY.Write([]byte{3}); err != nil {
		t.Errorf("Failed to send Ctrl+C: %v", err)
	}

	// Give it a moment to propagate
	time.Sleep(100 * time.Millisecond)

	// Close PTY/TTY pair
	if pt.Debug {
		t.Log("Closing PTY/TTY")
	}
	pt.PTY.Close()
	if pt.TTY != nil {
		pt.TTY.Close()
	}

	if pt.Debug {
		t.Log("PTY cleanup complete")
	}
}

// StartCommand starts a command with the PTY
func (pt *PTYTest) StartCommand(t *testing.T, cmd *exec.Cmd) error {
	var err error
	pt.PTY, err = pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start command with PTY: %v", err)
	}
	pt.Cmd = cmd
	return nil
}

// GetFreePort returns a random available port
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
