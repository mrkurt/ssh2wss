package client

import (
	"fmt"

	"golang.org/x/crypto/ssh"
)

// PTYSession defines the interface needed for PTY operations
type PTYSession interface {
	RequestPty(term string, h, w int, modes ssh.TerminalModes) error
	Shell() error
	WindowChange(h, w int) error
	Close() error
}

// PTY manages the remote pseudo-terminal
type PTY struct {
	session  PTYSession
	termType string
	width    int
	height   int
	modes    ssh.TerminalModes
}

// NewPTY creates a new PTY with the given session
func NewPTY(session PTYSession, termType string) *PTY {
	return &PTY{
		session:  session,
		termType: termType,
	}
}

// Start requests and starts the remote PTY
func (p *PTY) Start(width, height int, modes ssh.TerminalModes) error {
	p.width = width
	p.height = height
	p.modes = modes

	if err := p.session.RequestPty(p.termType, height, width, modes); err != nil {
		return fmt.Errorf("PTY request failed: %w", err)
	}

	if err := p.session.Shell(); err != nil {
		return fmt.Errorf("shell request failed: %w", err)
	}

	return nil
}

// Resize updates the PTY size
func (p *PTY) Resize(width, height int) error {
	if width == p.width && height == p.height {
		return nil // No change
	}

	p.width = width
	p.height = height

	if err := p.session.WindowChange(height, width); err != nil {
		return fmt.Errorf("failed to send window change: %w", err)
	}

	return nil
}

// Close cleans up the PTY
func (p *PTY) Close() error {
	return p.session.Close()
}
