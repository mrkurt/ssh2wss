package server

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

// PTYRequest represents a request for a PTY
type PTYRequest struct {
	Term   string
	Width  uint32
	Height uint32
	Modes  ssh.TerminalModes
}

// PTY manages a pseudo-terminal instance
type PTY struct {
	Command *exec.Cmd
	PTY     *os.File
	TTY     *os.File
}

// New creates a new PTY instance
func New(shell string) (*PTY, error) {
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm")

	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open pty: %w", err)
	}

	return &PTY{
		Command: cmd,
		PTY:     ptmx,
		TTY:     tty,
	}, nil
}

// Start starts the command in the PTY
func (p *PTY) Start() error {
	p.Command.Stdout = p.TTY
	p.Command.Stdin = p.TTY
	p.Command.Stderr = p.TTY
	p.Command.SysProcAttr = nil

	if err := p.Command.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	return nil
}

// Resize changes the size of the PTY window
func (p *PTY) Resize(width, height uint32) error {
	return pty.Setsize(p.PTY, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
}

// Close cleans up the PTY resources
func (p *PTY) Close() error {
	if err := p.TTY.Close(); err != nil {
		return fmt.Errorf("failed to close tty: %w", err)
	}
	if err := p.PTY.Close(); err != nil {
		return fmt.Errorf("failed to close pty: %w", err)
	}
	return nil
}
