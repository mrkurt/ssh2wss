//go:build windows
// +build windows

package server

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWindowsShell(t *testing.T) {
	shell, err := NewShell(80, 24)
	if err != nil {
		t.Fatalf("Failed to create shell: %v", err)
	}
	defer shell.Close()

	// Test basic echo command
	t.Run("Echo Command", func(t *testing.T) {
		err := shell.Start("cmd.exe")
		if err != nil {
			t.Fatalf("Failed to start shell: %v", err)
		}

		// Write echo command
		command := "echo hello world\r\n"
		_, err = shell.Write([]byte(command))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}

		// Read response
		buf := make([]byte, 1024)
		deadline := time.Now().Add(5 * time.Second)
		var output []byte

		for time.Now().Before(deadline) {
			n, err := shell.Read(buf)
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}
			output = append(output, buf[:n]...)
			if bytes.Contains(output, []byte("hello world")) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if !bytes.Contains(output, []byte("hello world")) {
			t.Errorf("Expected output to contain 'hello world', got: %q", output)
		}
	})

	// Test command execution
	t.Run("Direct Command", func(t *testing.T) {
		shell, err := NewShell(80, 24)
		if err != nil {
			t.Fatalf("Failed to create shell: %v", err)
		}
		defer shell.Close()

		err = shell.Start("cmd.exe /c echo test")
		if err != nil {
			t.Fatalf("Failed to start command: %v", err)
		}

		buf := make([]byte, 1024)
		deadline := time.Now().Add(5 * time.Second)
		var output []byte

		for time.Now().Before(deadline) {
			n, err := shell.Read(buf)
			if err != nil {
				break // EOF is expected
			}
			output = append(output, buf[:n]...)
			if bytes.Contains(output, []byte("test")) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if !bytes.Contains(output, []byte("test")) {
			t.Errorf("Expected output to contain 'test', got: %q", output)
		}
	})

	// Test window size
	t.Run("Window Size", func(t *testing.T) {
		shell, err := NewShell(100, 30)
		if err != nil {
			t.Fatalf("Failed to create shell: %v", err)
		}
		defer shell.Close()

		width, height := shell.WindowSize()
		if width != 100 || height != 30 {
			t.Errorf("Expected window size (100, 30), got (%d, %d)", width, height)
		}

		err = shell.Resize(120, 40)
		if err != nil {
			t.Fatalf("Failed to resize window: %v", err)
		}

		width, height = shell.WindowSize()
		if width != 120 || height != 40 {
			t.Errorf("Expected window size (120, 40), got (%d, %d)", width, height)
		}
	})

	// Test shell arguments
	t.Run("Shell Arguments", func(t *testing.T) {
		args := getShellArgs(false)
		if len(args) == 0 {
			t.Error("Expected non-empty shell arguments")
		}

		cmdArgs := getCommandArgs("dir")
		if len(cmdArgs) == 0 {
			t.Error("Expected non-empty command arguments")
		}

		if !strings.Contains(cmdArgs[len(cmdArgs)-1], "dir") {
			t.Errorf("Expected last argument to contain command, got: %v", cmdArgs)
		}
	})

	// Test exit codes
	t.Run("Exit Codes", func(t *testing.T) {
		tests := []struct {
			name     string
			command  string
			wantCode int
		}{
			{"Success", "cmd.exe /c exit 0", 0},
			{"Error", "cmd.exe /c exit 1", 1},
			{"Custom Code", "cmd.exe /c exit 42", 42},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				shell, err := NewShell(80, 24)
				if err != nil {
					t.Fatalf("Failed to create shell: %v", err)
				}

				err = shell.Start(tt.command)
				if err != nil {
					t.Fatalf("Failed to start command: %v", err)
				}

				// Read any output to ensure command completes
				buf := make([]byte, 1024)
				for {
					_, err := shell.Read(buf)
					if err != nil {
						break // EOF is expected
					}
				}

				// Close shell and check exit code
				shell.Close()
				gotCode := shell.GetExitCode()
				if gotCode != tt.wantCode {
					t.Errorf("GetExitCode() = %v, want %v", gotCode, tt.wantCode)
				}
			})
		}
	})
}

func TestWindowsExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		wantCode int
	}{
		// Internal commands
		{"dir success", "dir", 0},
		{"dir failure", "dir /nonexistentflag", 1},
		{"echo success", "echo test", 0},
		{"cd success", "cd .", 0},
		{"cd failure", "cd /nonexistent", 1},

		// External commands
		{"whoami success", "whoami", 0},
		{"nonexistent command", "nonexistentcommand", 1},
		{"ping success", "ping -n 1 127.0.0.1", 0},
		{"ping failure", "ping -n 1 invalid.local", 1},

		// Complex commands
		{"multiple commands success", "echo test && dir", 0},
		{"multiple commands failure", "echo test && nonexistentcommand", 1},
		{"pipe success", "dir | find \"Windows\"", 0},
		{"pipe failure", "dir | find \"nonexistentstring\"", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := NewShell(80, 24)
			if err != nil {
				t.Fatalf("Failed to create shell: %v", err)
			}
			defer shell.Close()

			err = shell.Start(tt.command)
			if err != nil {
				t.Fatalf("Failed to start command: %v", err)
			}

			// Read all output to ensure command completes
			buf := make([]byte, 1024)
			for {
				_, err := shell.Read(buf)
				if err != nil {
					break // EOF is expected
				}
			}

			// Close shell and check exit code
			shell.Close()
			gotCode := shell.GetExitCode()

			if gotCode != tt.wantCode {
				t.Errorf("Command %q: got exit code %d, want %d", tt.command, gotCode, tt.wantCode)
			}
		})
	}
}

func TestWindowsInternalCommandDetection(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"dir", true},
		{"DIR", true}, // Test case insensitivity
		{"dir /w", true},
		{"echo test", true},
		{"cd ..", true},
		{"whoami", false},
		{"ping", false},
		{"notepad", false},
		{"dir | sort", true}, // Should detect dir as internal
		{"echo test && cd ..", true},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := isInternalCommand(tt.command)
			if got != tt.want {
				t.Errorf("isInternalCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
