package client

import (
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// Terminal manages the local terminal state
type Terminal struct {
	stdin    *os.File
	stdout   *os.File
	oldState *term.State
	width    int
	height   int
	modes    ssh.TerminalModes
}

// NewTerminal creates a Terminal using the given input/output
func NewTerminal(stdin, stdout *os.File) *Terminal {
	return &Terminal{
		stdin:  stdin,
		stdout: stdout,
	}
}

// Setup prepares the terminal for interactive use
func (t *Terminal) Setup() error {
	// Get current state
	fd := int(t.stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %w", err)
	}
	t.oldState = state

	// Get terminal size
	width, height, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}
	t.width = width
	t.height = height

	// Set essential terminal modes for interactive use
	t.modes = ssh.TerminalModes{
		ssh.ECHO:   1, // Display what we type
		ssh.ISIG:   1, // Enable signals (Ctrl+C, etc)
		ssh.ICANON: 1, // Enable line editing (arrows, etc)
		ssh.ICRNL:  1, // Map CR to NL on input
	}

	return nil
}

// Restore returns the terminal to its original state
func (t *Terminal) Restore() error {
	if t.oldState != nil {
		return term.Restore(int(t.stdin.Fd()), t.oldState)
	}
	return nil
}

// Size returns the current terminal dimensions
func (t *Terminal) Size() (width, height int) {
	return t.width, t.height
}

// UpdateSize gets the current terminal size and returns true if it changed
func (t *Terminal) UpdateSize() (changed bool, err error) {
	width, height, err := term.GetSize(int(t.stdin.Fd()))
	if err != nil {
		return false, fmt.Errorf("failed to get terminal size: %w", err)
	}

	changed = width != t.width || height != t.height
	t.width = width
	t.height = height
	return changed, nil
}

// Modes returns the current terminal modes
func (t *Terminal) Modes() ssh.TerminalModes {
	return t.modes
}

// IsTerminal returns true if the given file is a terminal
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
