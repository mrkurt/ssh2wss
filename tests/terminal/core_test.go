package terminal

import (
	"bytes"
	"context"
	"flyssh/server"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ANSI escape sequences
const (
	ESC = "\x1b"
	CSI = ESC + "["
)

// Basic terminal control sequences
var (
	CursorUp      = fmt.Sprintf("%sA", CSI)  // Move cursor up one line
	CursorDown    = fmt.Sprintf("%sB", CSI)  // Move cursor down one line
	CursorRight   = fmt.Sprintf("%sC", CSI)  // Move cursor right one column
	CursorLeft    = fmt.Sprintf("%sD", CSI)  // Move cursor left one column
	ClearScreen   = fmt.Sprintf("%s2J", CSI) // Clear entire screen
	CursorHome    = fmt.Sprintf("%sH", CSI)  // Move cursor to home position
	SaveCursor    = fmt.Sprintf("%ss", CSI)  // Save cursor position
	RestoreCursor = fmt.Sprintf("%su", CSI)  // Restore cursor position
)

// Color escape sequences
var (
	Red     = fmt.Sprintf("%s31m", CSI)
	Green   = fmt.Sprintf("%s32m", CSI)
	Yellow  = fmt.Sprintf("%s33m", CSI)
	Blue    = fmt.Sprintf("%s34m", CSI)
	Magenta = fmt.Sprintf("%s35m", CSI)
	Cyan    = fmt.Sprintf("%s36m", CSI)
	Reset   = fmt.Sprintf("%s0m", CSI)
)

var binPath string
var testLogBuffer *bytes.Buffer
var testLogger *log.Logger
var sshServer *server.SSHServer
var sshPort int

func init() {
	var err error
	binPath, err = filepath.Abs("../../cmd/flyssh/flyssh")
	if err != nil {
		panic(fmt.Sprintf("Failed to get absolute path: %v", err))
	}
}

func TestMain(m *testing.M) {
	// Set up test logger
	testLogBuffer = &bytes.Buffer{}
	testLogger = log.New(testLogBuffer, "", 0)

	// Generate test host key
	hostKey, err := server.GenerateHostKey()
	if err != nil {
		log.Fatalf("Failed to generate host key: %v", err)
	}

	// Get a free port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("Failed to get free port: %v", err)
	}
	sshPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create SSH server with test logger
	sshServer, err = server.NewSSHServer(sshPort, hostKey)
	if err != nil {
		log.Fatalf("Failed to create SSH server: %v", err)
	}
	sshServer.SetLogger(testLogger)

	// Start server in background
	go func() {
		if err := sshServer.Start(); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Run tests
	code := m.Run()

	// Cleanup - just let the server goroutine exit
	os.Exit(code)
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

// setupTestServer starts a server instance for testing
func setupTestServer(t *testing.T, sshPort, wsPort int) (context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Generate auth token
	token := "test-token-" + fmt.Sprint(time.Now().UnixNano())
	os.Setenv("WSS_AUTH_TOKEN", token)

	// Ensure bash is available
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for these tests")
	}
	os.Setenv("SHELL", bashPath)

	// Start server in background
	serverStarted := make(chan bool)
	go func() {
		cmd := exec.CommandContext(ctx, binPath,
			"server",
			"-dev",
			"--ssh-port", fmt.Sprintf("%d", sshPort),
			"--ws-port", fmt.Sprintf("%d", wsPort))
		cmd.Env = append(os.Environ(), fmt.Sprintf("WSS_AUTH_TOKEN=%s", token))
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			t.Errorf("Server failed: %v", err)
		}
	}()

	// Wait for server to be ready
	go func() {
		maxAttempts := 10
		for i := 0; i < maxAttempts; i++ {
			conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", sshPort))
			if err == nil {
				conn.Close()
				serverStarted <- true
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		serverStarted <- false
	}()

	select {
	case ready := <-serverStarted:
		if !ready {
			cancel()
			return nil, fmt.Errorf("server failed to start after multiple attempts")
		}
	case <-time.After(5 * time.Second):
		cancel()
		return nil, fmt.Errorf("timeout waiting for server to start")
	}

	return cancel, nil
}

// Helper function to join commands with proper shell syntax
func joinCommands(commands []string) string {
	// Escape special characters in commands
	escaped := make([]string, len(commands))
	for i, cmd := range commands {
		// Escape single quotes for bash -c
		cmd = strings.ReplaceAll(cmd, "'", "'\"'\"'")
		escaped[i] = cmd
	}
	return strings.Join(escaped, " && ")
}

// Helper function to execute a command with proper setup
func executeCommand(t *testing.T, commands []string) ([]byte, error) {
	// Join commands with proper escaping
	script := fmt.Sprintf("export TERM=xterm-256color; %s", strings.Join(commands, "; "))

	// Create command to execute via ssh
	cmd := exec.Command("ssh",
		"-tt", // Force TTY allocation
		"-p", fmt.Sprintf("%d", sshPort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "SendEnv=TERM LANG LC_*", // Forward environment variables
		"localhost",
		script)

	// Set environment variables
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"COLUMNS=80",
		"LINES=24")

	// Run command and capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Include stderr in error output
			return append(output, exitErr.Stderr...), err
		}
		return output, err
	}

	return output, nil
}

func TestColorOutput(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Red",
			commands: []string{
				fmt.Sprintf("echo -e '%sError%s'", Red, Reset),
			},
			want: "Error",
		},
		{
			name: "Green",
			commands: []string{
				fmt.Sprintf("echo -e '%sSuccess%s'", Green, Reset),
			},
			want: "Success",
		},
		{
			name: "Blue",
			commands: []string{
				fmt.Sprintf("echo -e '%sInfo%s'", Blue, Reset),
			},
			want: "Info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Logf("Server logs: %s", getServerLogs()) // Log server output on failure
				t.Fatalf("Command failed: %v", err)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Logf("Server logs: %s", getServerLogs()) // Log server output on failure
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}
		})
	}
}

func TestCursorMovement(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Basic Movement",
			commands: []string{
				"echo -e 'X\\nY'",                     // Write X and Y on separate lines
				fmt.Sprintf("echo -e '%s'", CursorUp), // Move up one line
				"echo -n Z",                           // Write Z (should overwrite Y)
			},
			want: "X\nZ",
		},
		{
			name: "Save/Restore",
			commands: []string{
				fmt.Sprintf("echo -e '%sMarker%s'", SaveCursor, RestoreCursor),
				"echo -n X", // Should write at saved position
			},
			want: "X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Logf("Server logs: %s", getServerLogs())
				t.Fatalf("Command failed: %v", err)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Logf("Server logs: %s", getServerLogs())
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}
		})
	}
}

func TestLineWrapping(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Basic Wrapping",
			commands: []string{
				"printf '\\033[?7h'", // Enable line wrapping
				"echo -n 'This is a very long line that should wrap automatically'",
			},
			want: "wrap automatically",
		},
		{
			name: "Wrapping Disabled",
			commands: []string{
				"printf '\\033[?7l'", // Disable line wrapping
				"echo -n 'This line should not wrap but truncate'",
				"printf '\\033[?7h'", // Re-enable wrapping
			},
			want: "truncate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Logf("Server logs: %s", getServerLogs())
				t.Fatalf("Command failed: %v", err)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Logf("Server logs: %s", getServerLogs())
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}
		})
	}
}

func TestBackspaceHandling(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Simple Backspace",
			commands: []string{
				"echo -n 'abc'",
				"printf '\\b'",
				"echo -n 'X'",
			},
			want: "abX",
		},
		{
			name: "Multiple Backspace",
			commands: []string{
				"echo -n 'abcd'",
				"printf '\\b\\b'",
				"echo -n 'XY'",
			},
			want: "abXY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Logf("Server logs: %s", getServerLogs())
				t.Fatalf("Command failed: %v", err)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Logf("Server logs: %s", getServerLogs())
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}
		})
	}
}

// Helper function to get server logs
func getServerLogs() string {
	return testLogBuffer.String()
}

// Clear server logs between tests
func clearServerLogs() {
	testLogBuffer.Reset()
}
