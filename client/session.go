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

	// Cleanup and control
	done       chan struct{}
	cleanup    []func() error
	resizeChan chan struct{} // Serialize resize events
}

// NewSession creates a Session ready for interactive use
func NewSession(conn sshClientInterface) (*Session, error) {
	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}

	s := &Session{
		conn:       conn,
		session:    session,
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		done:       make(chan struct{}),
		cleanup:    make([]func() error, 0),
		termType:   os.Getenv("TERM"),
		resizeChan: make(chan struct{}, 1), // Buffer one resize event
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

	// Set essential terminal modes for interactive use
	s.modes = ssh.TerminalModes{
		ssh.ECHO:   1, // Display what we type
		ssh.ISIG:   1, // Enable signals (Ctrl+C, etc)
		ssh.ICANON: 1, // Enable line editing (arrows, etc)
		ssh.ICRNL:  1, // Map CR to NL on input
	}

	// Start resize handler immediately after raw mode
	go s.resizeHandler()

	return nil
}

// setupSignals configures signal handling for the session
func (s *Session) setupSignals() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGWINCH, // Window size changes
		syscall.SIGTERM,  // Termination
		syscall.SIGINT,   // Interrupt (Ctrl+C)
		syscall.SIGQUIT,  // Quit (Ctrl+\)
	)

	go func() {
		for {
			select {
			case sig := <-sigChan:
				switch sig {
				case syscall.SIGWINCH:
					// Trigger resize without blocking
					select {
					case s.resizeChan <- struct{}{}:
					default:
						// Resize already pending
					}

				case syscall.SIGINT:
					// Forward interrupt
					s.forwardSignal(ssh.SIGINT)

				case syscall.SIGQUIT:
					// Forward quit
					s.forwardSignal(ssh.SIGQUIT)

				case syscall.SIGTERM:
					// Clean shutdown
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

// forwardSignal sends a signal to the remote process and handles errors
func (s *Session) forwardSignal(sig ssh.Signal) {
	if err := s.session.Signal(sig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to send signal %v: %v\n", sig, err)
	}
}

// resizeHandler processes window resize events serially
func (s *Session) resizeHandler() {
	for {
		select {
		case <-s.resizeChan:
			if err := s.handleResize(); err != nil {
				fmt.Fprintf(os.Stderr, "resize error: %v\n", err)
			}
		case <-s.done:
			return
		}
	}
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
