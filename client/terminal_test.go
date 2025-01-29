package client

import (
	"os"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestTerminalSetup(t *testing.T) {
	// Skip if not running in a terminal
	if !IsTerminal(os.Stdin) {
		t.Skip("Test requires a terminal")
	}

	term := NewTerminal(os.Stdin, os.Stdout)
	if err := term.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer term.Restore()

	// Verify terminal modes
	modes := term.Modes()
	requiredModes := []struct {
		mode uint8
		val  uint32
	}{
		{ssh.ECHO, 1},
		{ssh.ISIG, 1},
		{ssh.ICANON, 1},
		{ssh.ICRNL, 1},
	}

	for _, m := range requiredModes {
		if val := modes[m.mode]; val != m.val {
			t.Errorf("mode %v = %v, want %v", m.mode, val, m.val)
		}
	}

	// Verify we got terminal size
	width, height := term.Size()
	if width == 0 || height == 0 {
		t.Errorf("invalid terminal size: %dx%d", width, height)
	}
}

func TestTerminalSizeUpdate(t *testing.T) {
	// Skip if not running in a terminal
	if !IsTerminal(os.Stdin) {
		t.Skip("Test requires a terminal")
	}

	term := NewTerminal(os.Stdin, os.Stdout)
	if err := term.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer term.Restore()

	// Get initial size
	origWidth, origHeight := term.Size()

	// Update size (should be unchanged)
	changed, err := term.UpdateSize()
	if err != nil {
		t.Fatalf("UpdateSize failed: %v", err)
	}

	if changed {
		t.Error("UpdateSize reported change when size was unchanged")
	}

	width, height := term.Size()
	if width != origWidth || height != origHeight {
		t.Errorf("size changed unexpectedly: got %dx%d, want %dx%d",
			width, height, origWidth, origHeight)
	}
}

func TestTerminalRestore(t *testing.T) {
	// Skip if not running in a terminal
	if !IsTerminal(os.Stdin) {
		t.Skip("Test requires a terminal")
	}

	term := NewTerminal(os.Stdin, os.Stdout)

	// Restore without setup should not error
	if err := term.Restore(); err != nil {
		t.Errorf("Restore without setup failed: %v", err)
	}

	// Setup and restore should work
	if err := term.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if err := term.Restore(); err != nil {
		t.Errorf("Restore after setup failed: %v", err)
	}
}

func TestIsTerminal(t *testing.T) {
	// Skip if not running in a terminal
	if !IsTerminal(os.Stdin) {
		t.Skip("Test requires a terminal")
	}

	tests := []struct {
		name string
		file *os.File
		want bool
	}{
		{
			name: "stdin",
			file: os.Stdin,
			want: true,
		},
		{
			name: "temp file",
			file: func() *os.File {
				f, err := os.CreateTemp("", "terminal_test")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Remove(f.Name()) })
				return f
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminal(tt.file); got != tt.want {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}
