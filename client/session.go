package client

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// sshClientInterface allows us to use either real or mock clients
type sshClientInterface interface {
	NewSession() (*ssh.Session, error)
}

// Session manages an SSH session
type Session struct {
	conn    sshClientInterface
	session *ssh.Session
	done    chan struct{}
	cleanup []func() error
}

// NewSession creates a new SSH session
func NewSession(conn sshClientInterface) (*Session, error) {
	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}

	s := &Session{
		conn:    conn,
		session: session,
		done:    make(chan struct{}),
		cleanup: make([]func() error, 0),
	}

	s.cleanup = append(s.cleanup, session.Close)
	return s, nil
}

// Start begins an interactive session
func (s *Session) Start() error {
	// Set up I/O
	s.session.Stdin = os.Stdin
	s.session.Stdout = os.Stdout
	s.session.Stderr = os.Stderr

	// Request PTY if we're running on a terminal
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if err := s.setupPTY(); err != nil {
			return fmt.Errorf("PTY setup failed: %w", err)
		}
	}

	// Start remote shell
	if err := s.session.Shell(); err != nil {
		return fmt.Errorf("shell request failed: %w", err)
	}

	// Wait for session to complete
	return s.session.Wait()
}

// setupPTY configures the PTY for the session
func (s *Session) setupPTY() error {
	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm"
	}

	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.IGNCR:         0,
		ssh.ICANON:        1,
		ssh.ISIG:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := s.session.RequestPty(termType, height, width, modes); err != nil {
		return fmt.Errorf("PTY request failed: %w", err)
	}

	// Set up signal handling for window resizes
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)
	go s.handleWindowChanges(sigChan)

	return nil
}

// handleWindowChanges manages terminal window size changes
func (s *Session) handleWindowChanges(sigChan chan os.Signal) {
	for {
		select {
		case <-sigChan:
			width, height, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				continue
			}
			s.session.WindowChange(height, width)
		case <-s.done:
			return
		}
	}
}

// Close cleans up session resources
func (s *Session) Close() error {
	close(s.done)
	var lastErr error
	for i := len(s.cleanup) - 1; i >= 0; i-- {
		if err := s.cleanup[i](); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
