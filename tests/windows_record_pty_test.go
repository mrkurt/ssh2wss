//go:build manual
// +build manual

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestRecordPTY records a real PTY session for later playback
// Run with: go test -tags=manual -v -run TestRecordPTY
func TestRecordPTY(t *testing.T) {
	// Get absolute path for fixtures
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	fixturePath := filepath.Join(wd, "fixtures", "pty_recording.json")
	t.Logf("Will write recording to: %s", fixturePath)

	// Start a real PTY with a non-interactive shell to avoid startup scripts
	cmd := exec.Command("bash", "--norc", "--noprofile")
	cmd.Env = []string{
		"TERM=xterm",
		"PS1=$ ",
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("Failed to start PTY: %v", err)
	}

	// Ensure cleanup happens
	defer func() {
		ptmx.Write([]byte("exit\n"))
		time.Sleep(100 * time.Millisecond)
		ptmx.Close()
		if err := cmd.Wait(); err != nil {
			fmt.Printf("Warning: PTY cleanup error: %v\n", err)
		}
	}()

	// Set initial window size
	size := WindowSize{Rows: 24, Cols: 80}
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: size.Rows,
		Cols: size.Cols,
	}); err != nil {
		t.Fatalf("Failed to set window size: %v", err)
	}

	// Wait for initial prompt with timeout
	buf := make([]byte, 1024)
	promptFound := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, err := ptmx.Read(buf)
		if err != nil {
			t.Logf("Warning: read error while waiting for prompt: %v", err)
			continue
		}
		if n > 0 && bytes.Contains(buf[:n], []byte("$ ")) {
			promptFound = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !promptFound {
		t.Fatal("Timed out waiting for initial prompt")
	}

	// Simple test cases that should work on any shell
	commands := []struct {
		input      string
		delayAfter float64
	}{
		{"echo hello\n", 0.1},
		{"echo -e '\\033[31mred\\033[0m'\n", 0.1},
	}

	recording := PTYRecording{
		Interactions: make([]PTYInteraction, 0, len(commands)),
	}

	// Run each command and record input/output
	for i, cmd := range commands {
		t.Logf("Running command %d: %q", i+1, cmd.input)

		interaction := PTYInteraction{
			Input:      []byte(cmd.input),
			WindowSize: size,
			Delay:      cmd.delayAfter,
		}

		// Send command
		if _, err := ptmx.Write([]byte(cmd.input)); err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}

		// Read response with timeout
		output := &bytes.Buffer{}
		promptFound = false
		deadline = time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			n, err := ptmx.Read(buf)
			if err != nil {
				t.Logf("Warning: read error while waiting for output: %v", err)
				continue
			}
			if n > 0 {
				output.Write(buf[:n])
				if bytes.Contains(buf[:n], []byte("$ ")) {
					promptFound = true
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !promptFound {
			t.Fatalf("Timed out waiting for command %d output", i+1)
		}

		interaction.Output = output.Bytes()
		recording.Interactions = append(recording.Interactions, interaction)
	}

	// Let Go handle all the JSON encoding
	recordingJSON, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal recording: %v", err)
	}

	// Create fixtures directory if it doesn't exist
	fixtureDir := filepath.Dir(fixturePath)
	if err := os.MkdirAll(fixtureDir, 0755); err != nil {
		t.Fatalf("Failed to create fixtures directory: %v", err)
	}

	// Write the recording
	if err := os.WriteFile(fixturePath, recordingJSON, 0644); err != nil {
		t.Fatalf("Failed to write recording: %v", err)
	}

	t.Logf("Successfully recorded %d PTY interactions to %s", len(recording.Interactions), fixturePath)
}
