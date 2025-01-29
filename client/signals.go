package client

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
)

// SignalSession defines the interface needed for signal handling
type SignalSession interface {
	Signal(sig ssh.Signal) error
	WindowChange(h, w int) error
}

// TerminalSize defines the interface needed for terminal resizing
type TerminalSize interface {
	UpdateSize() (bool, error)
	Size() (width, height int)
}

// SignalHandler manages signal handling for a session
type SignalHandler struct {
	session    SignalSession
	done       chan struct{}
	resizeChan chan struct{}
	terminal   TerminalSize
}

// NewSignalHandler creates a SignalHandler for the given session
func NewSignalHandler(session SignalSession, terminal TerminalSize) *SignalHandler {
	return &SignalHandler{
		session:    session,
		done:       make(chan struct{}),
		resizeChan: make(chan struct{}, 1), // Buffer one resize event
		terminal:   terminal,
	}
}

// Start begins signal handling
func (h *SignalHandler) Start() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGWINCH, // Window size changes
		syscall.SIGTERM,  // Termination
		syscall.SIGINT,   // Interrupt (Ctrl+C)
		syscall.SIGQUIT,  // Quit (Ctrl+\)
	)

	go h.handleSignals(sigChan)
	go h.handleResize()

	return nil
}

// Stop stops signal handling
func (h *SignalHandler) Stop() {
	close(h.done)
}

// handleSignals processes incoming signals
func (h *SignalHandler) handleSignals(sigChan chan os.Signal) {
	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGWINCH:
				// Trigger resize without blocking
				select {
				case h.resizeChan <- struct{}{}:
				default:
					// Resize already pending
				}

			case syscall.SIGINT:
				h.forwardSignal(ssh.SIGINT)

			case syscall.SIGQUIT:
				h.forwardSignal(ssh.SIGQUIT)

			case syscall.SIGTERM:
				h.Stop()
				return
			}
		case <-h.done:
			return
		}
	}
}

// handleResize processes window resize events
func (h *SignalHandler) handleResize() {
	for {
		select {
		case <-h.resizeChan:
			if err := h.updateWindowSize(); err != nil {
				fmt.Fprintf(os.Stderr, "resize error: %v\n", err)
			}
		case <-h.done:
			return
		}
	}
}

// updateWindowSize updates the terminal size and notifies the remote
func (h *SignalHandler) updateWindowSize() error {
	changed, err := h.terminal.UpdateSize()
	if err != nil {
		return err
	}

	if !changed {
		return nil
	}

	width, height := h.terminal.Size()
	if err := h.session.WindowChange(height, width); err != nil {
		return fmt.Errorf("failed to send window change: %w", err)
	}

	return nil
}

// forwardSignal sends a signal to the remote process
func (h *SignalHandler) forwardSignal(sig ssh.Signal) {
	if err := h.session.Signal(sig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to send signal %v: %v\n", sig, err)
	}
}
