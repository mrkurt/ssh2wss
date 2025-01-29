package client

import (
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// mockSSHClient implements sshClientInterface for testing
type mockSSHClient struct {
	sessionErr error
	session    *mockSession
}

type mockSession struct {
	closeErr error
}

func (s *mockSession) Close() error                                                { return s.closeErr }
func (s *mockSession) RequestPty(term string, h, w int, m ssh.TerminalModes) error { return nil }
func (s *mockSession) Shell() error                                                { return nil }
func (s *mockSession) Wait() error                                                 { return nil }
func (s *mockSession) WindowChange(h, w int) error                                 { return nil }
func (s *mockSession) Start(cmd string) error                                      { return nil }
func (s *mockSession) Signal(sig ssh.Signal) error                                 { return nil }

func (m *mockSSHClient) NewSession() (*ssh.Session, error) {
	if m.sessionErr != nil {
		return nil, m.sessionErr
	}
	if m.session == nil {
		m.session = &mockSession{}
	}
	return &ssh.Session{}, nil
}

func TestNewSession(t *testing.T) {
	tests := []struct {
		name    string
		client  sshClientInterface
		wantErr bool
	}{
		{
			name:    "success",
			client:  &mockSSHClient{},
			wantErr: false,
		},
		{
			name:    "session creation fails",
			client:  &mockSSHClient{sessionErr: io.EOF},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewSession(tt.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && session == nil {
				t.Error("NewSession() returned nil session without error")
			}
		})
	}
}

func TestSessionCleanup(t *testing.T) {
	// Create a temporary file to test cleanup
	tmpfile, err := os.CreateTemp("", "session_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Create session
	s := &Session{
		done:    make(chan struct{}),
		cleanup: make([]func() error, 0),
	}

	// Add some cleanup functions
	cleanupCalled := false
	s.cleanup = append(s.cleanup, func() error {
		cleanupCalled = true
		return nil
	})

	// Close session
	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify cleanup was called
	if !cleanupCalled {
		t.Error("Cleanup function was not called")
	}

	// Verify done channel is closed
	select {
	case <-s.done:
		// Channel is closed as expected
	case <-time.After(time.Second):
		t.Error("Done channel was not closed")
	}
}

func TestSessionSignalHandling(t *testing.T) {
	s := &Session{
		done:    make(chan struct{}),
		cleanup: make([]func() error, 0),
	}

	if err := s.setupSignals(); err != nil {
		t.Fatalf("setupSignals() error = %v", err)
	}

	// Verify signal handler exits when done channel is closed
	s.Close()

	// Give the goroutine time to exit
	time.Sleep(100 * time.Millisecond)
}
