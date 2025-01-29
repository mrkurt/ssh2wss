package client

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type signalTestSession struct {
	signals     []ssh.Signal
	windowCalls int
	signalErr   error
	windowErr   error
}

func (m *signalTestSession) Signal(sig ssh.Signal) error {
	if m.signalErr != nil {
		return m.signalErr
	}
	m.signals = append(m.signals, sig)
	return nil
}

func (m *signalTestSession) WindowChange(h, w int) error {
	if m.windowErr != nil {
		return m.windowErr
	}
	m.windowCalls++
	return nil
}

type mockTerminal struct {
	width   int
	height  int
	sizeErr error
	changed bool
}

func (t *mockTerminal) UpdateSize() (bool, error) {
	if t.sizeErr != nil {
		return false, t.sizeErr
	}
	return t.changed, nil
}

func (t *mockTerminal) Size() (width, height int) {
	return t.width, t.height
}

func TestSignalHandling(t *testing.T) {
	// Create mock session and terminal
	mock := &signalTestSession{}
	term := NewTerminal(os.Stdin, os.Stdout)
	handler := NewSignalHandler(mock, term)

	// Start signal handling
	if err := handler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Test signal forwarding
	tests := []struct {
		name string
		sig  os.Signal
		want ssh.Signal
	}{
		{"interrupt", syscall.SIGINT, ssh.SIGINT},
		{"quit", syscall.SIGQUIT, ssh.SIGQUIT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.signals = nil // Reset signals

			// Send signal
			syscall.Kill(os.Getpid(), tt.sig.(syscall.Signal))

			// Wait for signal to be processed
			time.Sleep(100 * time.Millisecond)

			// Verify signal was forwarded
			if len(mock.signals) != 1 {
				t.Errorf("got %d signals, want 1", len(mock.signals))
			} else if mock.signals[0] != tt.want {
				t.Errorf("got signal %v, want %v", mock.signals[0], tt.want)
			}
		})
	}

	// Test cleanup
	handler.Stop()
	time.Sleep(100 * time.Millisecond) // Wait for goroutines to exit
}

func TestResizeHandling(t *testing.T) {
	tests := []struct {
		name       string
		session    *signalTestSession
		terminal   *mockTerminal
		wantResize bool
		wantErr    bool
	}{
		{
			name:    "no change",
			session: &signalTestSession{},
			terminal: &mockTerminal{
				width:   80,
				height:  24,
				changed: false,
			},
			wantResize: false,
		},
		{
			name:    "size changes",
			session: &signalTestSession{},
			terminal: &mockTerminal{
				width:   100,
				height:  30,
				changed: true,
			},
			wantResize: true,
		},
		{
			name:    "resize error",
			session: &signalTestSession{windowErr: errors.New("resize failed")},
			terminal: &mockTerminal{
				width:   100,
				height:  30,
				changed: true,
			},
			wantResize: false, // No window change call should succeed
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewSignalHandler(tt.session, tt.terminal)

			// Start signal handling
			if err := handler.Start(); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			defer handler.Stop()

			// Send resize event
			handler.resizeChan <- struct{}{}

			// Wait for resize to be processed
			time.Sleep(100 * time.Millisecond)

			// Verify window change was called if size changed
			if tt.wantResize && tt.session.windowCalls != 1 {
				t.Errorf("got %d window changes, want 1", tt.session.windowCalls)
			}
			if !tt.wantResize && tt.session.windowCalls != 0 {
				t.Errorf("got %d window changes, want 0", tt.session.windowCalls)
			}
		})
	}
}
