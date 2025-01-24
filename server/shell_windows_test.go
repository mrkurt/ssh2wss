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
}
