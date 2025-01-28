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

// Session manages an interactive SSH terminal session
type Session struct {
	// Core SSH connection
	conn    sshClientInterface
	session *ssh.Session

	// Terminal state
	stdin    *os.File
	stdout   *os.File
	oldState *term.State

	// Dimensions and modes
	termType string // e.g. "xterm-256color"
	width    int
	height   int
	modes    ssh.TerminalModes

	// Cleanup
	done    chan struct{}
	cleanup []func() error
}

// NewSession creates a Session ready for interactive use
func NewSession(conn sshClientInterface) (*Session, error) {
	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}

	s := &Session{
		conn:     conn,
		session:  session,
		stdin:    os.Stdin,
		stdout:   os.Stdout,
		done:     make(chan struct{}),
		cleanup:  make([]func() error, 0),
		termType: os.Getenv("TERM"),
	}

	// Add session cleanup to the list
	s.cleanup = append(s.cleanup, session.Close)

	return s, nil
}

// Start begins an interactive terminal session
func (s *Session) Start() error {
	if err := s.setupTerminal(); err != nil {
		return fmt.Errorf("terminal setup failed: %w", err)
	}
	defer s.Close()

	if err := s.setupSignals(); err != nil {
		return fmt.Errorf("signal setup failed: %w", err)
	}

	// Request PTY with current terminal dimensions
	if err := s.session.RequestPty(s.termType, s.height, s.width, s.modes); err != nil {
		return fmt.Errorf("PTY request failed: %w", err)
	}

	// Set up pipes for stdin/stdout/stderr
	s.session.Stdin = s.stdin
	s.session.Stdout = s.stdout
	s.session.Stderr = os.Stderr

	// Start remote shell
	if err := s.session.Shell(); err != nil {
		return fmt.Errorf("shell request failed: %w", err)
	}

	// Wait for session to complete or signal
	return s.session.Wait()
}

// Close cleans up resources and restores terminal state
func (s *Session) Close() error {
	// Signal we're done
	close(s.done)

	// Run cleanup functions in reverse order
	var lastErr error
	for i := len(s.cleanup) - 1; i >= 0; i-- {
		if err := s.cleanup[i](); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// setupTerminal prepares the local terminal for interactive use
func (s *Session) setupTerminal() error {
	// Get current state
	fd := int(s.stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %w", err)
	}

	// Store state and add cleanup
	s.oldState = state
	s.cleanup = append(s.cleanup, func() error {
		return term.Restore(fd, state)
	})

	// Get terminal size
	width, height, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	s.width = width
	s.height = height

	// Set basic terminal modes
	s.modes = ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	return nil
}

// setupSignals configures signal handling for the session
func (s *Session) setupSignals() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGWINCH)

	go func() {
		for {
			select {
			case sig := <-sigChan:
				switch sig {
				case syscall.SIGWINCH:
					s.handleResize()
				case syscall.SIGTERM, syscall.SIGINT:
					s.Close()
					return
				}
			case <-s.done:
				return
			}
		}
	}()

	return nil
}

// handleResize updates terminal dimensions when the window size changes
func (s *Session) handleResize() error {
	width, height, err := term.GetSize(int(s.stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	if width == s.width && height == s.height {
		return nil // No change
	}

	s.width = width
	s.height = height

	if err := s.session.WindowChange(height, width); err != nil {
		return fmt.Errorf("failed to send window change: %w", err)
	}

	return nil
}
