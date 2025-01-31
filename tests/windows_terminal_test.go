//go:build windows
// +build windows

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"encoding/base64"

	"golang.org/x/sys/windows"
)

var (
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	createPseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	closePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
)

// windowsConsole represents a real Windows console
type windowsConsole interface {
	io.ReadWriter
	Close() error
}

// simulatedPTY represents a simulated PTY (either client or server)
type simulatedPTY interface {
	io.ReadWriter
	Reset()
}

// newWindowsConsole creates a real Windows console
// On non-Windows platforms, this is a stub that returns an error
func newWindowsConsole() (windowsConsole, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("Windows console not supported on %s", runtime.GOOS)
	}
	// TODO: Implement real Windows console creation
	return nil, fmt.Errorf("not implemented")
}

// simulatedServerPTY simulates a Unix PTY on the server side
type simulatedServerPTY struct {
	input     *bytes.Buffer
	output    *bytes.Buffer
	mu        sync.Mutex
	rows      uint16
	cols      uint16
	rawMode   bool
	lastWrite []byte // for debugging
}

// Ensure simulatedServerPTY implements simulatedPTY
var _ simulatedPTY = (*simulatedServerPTY)(nil)

func newSimulatedServerPTY() *simulatedServerPTY {
	return &simulatedServerPTY{
		input:  &bytes.Buffer{},
		output: &bytes.Buffer{},
		rows:   24,
		cols:   80,
	}
}

// Write implements io.Writer - simulates Unix PTY output
func (s *simulatedServerPTY) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastWrite = make([]byte, len(p))
	copy(s.lastWrite, p)

	// Process as Unix PTY would
	processed := s.processUnixStyle(p)

	// Write to output buffer only - this is what the server sees
	if _, err := s.output.Write(processed); err != nil {
		return 0, err
	}

	// Echo back to input buffer - this is what gets sent back to client
	if _, err := s.input.Write(processed); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Read implements io.Reader - reads from input buffer
func (s *simulatedServerPTY) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err = s.input.Read(p)
	if n > 0 {
		fmt.Printf("Server PTY Read: %q\n", p[:n])
	}
	return n, err
}

// Reset clears the buffers
func (s *simulatedServerPTY) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input.Reset()
	s.output.Reset()
}

// processUnixStyle handles Unix PTY behaviors
func (s *simulatedServerPTY) processUnixStyle(p []byte) []byte {
	fmt.Printf("Server PTY processing: %q\n", p)
	// Convert Windows line endings to Unix
	p = bytes.ReplaceAll(p, []byte("\r\n"), []byte("\n"))

	// Pass through ANSI sequences (Unix PTYs support them natively)
	return p
}

// readInput reads directly from input buffer (for testing)
func (s *simulatedServerPTY) readInput(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.Read(p)
}

// readOutput reads directly from output buffer (for testing)
func (s *simulatedServerPTY) readOutput(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.Read(p)
}

// simulatedClientPTY simulates a Windows terminal
type simulatedClientPTY struct {
	input     *bytes.Buffer
	output    *bytes.Buffer
	mu        sync.Mutex
	rows      uint16
	cols      uint16
	rawMode   bool
	lastWrite []byte // for debugging
}

func newSimulatedPTY() *simulatedClientPTY {
	return &simulatedClientPTY{
		input:  &bytes.Buffer{},
		output: &bytes.Buffer{},
		rows:   24,
		cols:   80,
	}
}

// Write implements io.Writer - simulates Windows console output
func (s *simulatedClientPTY) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastWrite = make([]byte, len(p))
	copy(s.lastWrite, p)

	// Process as Windows console would
	processed := s.processWindowsStyle(p)

	// Write processed output
	n, err = s.output.Write(processed)
	return len(p), err // return original length even if we modified the output
}

// Read implements io.Reader
func (s *simulatedClientPTY) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.Read(p)
}

// Reset clears the buffers
func (s *simulatedClientPTY) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input.Reset()
	s.output.Reset()
}

// processWindowsStyle handles Windows console behaviors
func (s *simulatedClientPTY) processWindowsStyle(p []byte) []byte {
	// Convert Unix line endings to Windows
	p = bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))

	// Strip ANSI sequences (Windows console doesn't support them by default)
	p = stripANSI(p)

	return p
}

// stripANSI removes ANSI escape sequences from the input
func stripANSI(p []byte) []byte {
	var result []byte
	for i := 0; i < len(p); i++ {
		if p[i] == '\x1b' {
			// Skip until end of sequence
			for i++; i < len(p); i++ {
				if (p[i] >= 'A' && p[i] <= 'Z') || (p[i] >= 'a' && p[i] <= 'z') {
					break
				}
			}
			continue
		}
		result = append(result, p[i])
	}
	return result
}

// TestClientWindows tests the Windows terminal behavior against a simulated Unix PTY server.
// On Windows, it uses the real Windows console.
// On non-Windows, it uses an emulated Windows console.
func TestClientWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		if os.Getenv("CI") != "" {
			t.Skip("Skipping on CI - requires real Windows console")
		}
		runWindowsTerminalTest(t, false) // Use real console
	} else {
		runWindowsTerminalTest(t, true) // Use emulation
	}
}

// TestClientWindowsEmulated tests Windows terminal behavior using emulation
// This test runs only on non-Windows platforms
func TestClientWindowsEmulated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping emulated terminal test on Windows")
	}
	runWindowsTerminalTest(t, true)
}

// TestClientWindowsReal tests Windows terminal behavior using the real console
// This test runs only on Windows
func TestClientWindowsReal(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping on CI - requires real Windows console")
	}
	runWindowsTerminalTest(t, false)
}

// runWindowsTerminalTest contains the actual test logic
func runWindowsTerminalTest(t *testing.T, useEmulation bool) {
	// Create simulated Unix PTY server
	serverPTY := newSimulatedServerPTY()

	// Create Windows terminal - real or emulated based on parameter
	var clientTerm io.ReadWriter
	if !useEmulation {
		// Use real Windows console
		term, err := newWindowsConsole()
		if err != nil {
			t.Fatal("Failed to create Windows console:", err)
		}
		defer term.Close()
		clientTerm = term
	} else {
		// Use emulated Windows console
		clientTerm = newSimulatedPTY()
	}

	// Create communication channels
	clientToServer := make(chan []byte, 100)
	serverToClient := make(chan []byte, 100)
	defer close(clientToServer)
	defer close(serverToClient)

	// Create context with timeout for each test case
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start server handler
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-clientToServer:
				if !ok {
					return
				}
				// Process client input through server PTY
				if _, err := serverPTY.Write(data); err != nil {
					t.Errorf("Server PTY write error: %v", err)
					return
				}

				// Read server PTY output
				n, err := serverPTY.Read(buf)
				if err != nil && err != io.EOF {
					t.Errorf("Server PTY read error: %v", err)
					return
				}

				// Send back to client
				if n > 0 {
					select {
					case <-ctx.Done():
						return
					case serverToClient <- buf[:n]:
					}
				}
			}
		}
	}()

	// Load and decode test cases from recording
	recording, err := loadPTYRecording(t)
	if err != nil {
		t.Fatal("Failed to load PTY recording:", err)
	}

	// Run recorded test cases
	for i, interaction := range recording.Interactions {
		name := fmt.Sprintf("recorded_interaction_%d", i)
		t.Run(name, func(t *testing.T) {
			// Clear previous output
			if clearable, ok := clientTerm.(simulatedPTY); ok {
				clearable.Reset()
			}
			serverPTY.Reset()

			// Set window size if specified
			if interaction.WindowSize.Rows > 0 && interaction.WindowSize.Cols > 0 {
				serverPTY.rows = interaction.WindowSize.Rows
				serverPTY.cols = interaction.WindowSize.Cols
			}

			// Send recorded input
			select {
			case <-ctx.Done():
				t.Fatal("Test timed out")
			case clientToServer <- interaction.Input:
			}

			// Process server response
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				t.Fatal("Test timed out")
			case data := <-serverToClient:
				if _, err := clientTerm.Write(data); err != nil {
					t.Fatalf("Failed to write to client: %v", err)
				}
			case <-timer.C:
				t.Fatal("Timed out waiting for server response")
			}

			// Read and verify client output
			clientBuf := make([]byte, 1024)
			n, err := clientTerm.Read(clientBuf)
			if err != nil && err != io.EOF {
				t.Fatalf("Failed to read client output: %v", err)
			}

			// Windows-specific output transformation
			expectedOutput := processWindowsOutput(interaction.Output)
			if !bytes.Contains(clientBuf[:n], expectedOutput) {
				t.Errorf("Client expected %q, got %q", expectedOutput, clientBuf[:n])
			}
		})

		// Respect recorded delay between interactions
		time.Sleep(time.Duration(interaction.Delay * float64(time.Second)))
	}

	// Clean shutdown
	cancel()
	wg.Wait()
}

// NOTE: The following types are duplicated from pty_recording.go
// This duplication is necessary because this file is conditionally built with //go:build windows
// while pty_recording.go is used for non-Windows builds. Both files need these types
// but can't share them due to build constraints.

// PTYInteraction represents a single recorded terminal interaction
type PTYInteraction struct {
	Input      []byte  `json:"input"`
	Output     []byte  `json:"output"`
	Delay      float64 `json:"delay"`
	WindowSize struct {
		Rows uint16 `json:"rows"`
		Cols uint16 `json:"cols"`
	} `json:"windowSize"`
}

// PTYRecording represents a recorded terminal session
type PTYRecording struct {
	Interactions []PTYInteraction `json:"interactions"`
}

// loadPTYRecording loads and decodes the test recording
func loadPTYRecording(t *testing.T) (*PTYRecording, error) {
	data, err := os.ReadFile("tests/fixtures/pty_recording.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read recording: %v", err)
	}

	var recording struct {
		Interactions []struct {
			Input      string  `json:"input"`
			Output     string  `json:"output"`
			Delay      float64 `json:"delay"`
			WindowSize struct {
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			} `json:"windowSize"`
		} `json:"interactions"`
	}

	if err := json.Unmarshal(data, &recording); err != nil {
		return nil, fmt.Errorf("failed to parse recording: %v", err)
	}

	result := &PTYRecording{
		Interactions: make([]PTYInteraction, len(recording.Interactions)),
	}

	for i, interaction := range recording.Interactions {
		input, err := base64.StdEncoding.DecodeString(interaction.Input)
		if err != nil {
			return nil, fmt.Errorf("failed to decode input %d: %v", i, err)
		}
		output, err := base64.StdEncoding.DecodeString(interaction.Output)
		if err != nil {
			return nil, fmt.Errorf("failed to decode output %d: %v", i, err)
		}
		result.Interactions[i].Input = input
		result.Interactions[i].Output = output
		result.Interactions[i].Delay = interaction.Delay
		result.Interactions[i].WindowSize.Rows = interaction.WindowSize.Rows
		result.Interactions[i].WindowSize.Cols = interaction.WindowSize.Cols
	}

	return result, nil
}

// processWindowsOutput transforms recorded Unix output to expected Windows output
func processWindowsOutput(unixOutput []byte) []byte {
	// Convert Unix line endings to Windows
	output := bytes.ReplaceAll(unixOutput, []byte("\n"), []byte("\r\n"))

	// Strip ANSI sequences for Windows
	output = stripANSI(output)

	return output
}
