package client

import (
	"errors"
	"testing"

	"golang.org/x/crypto/ssh"
)

type ptyTestSession struct {
	ptyRequested  bool
	shellStarted  bool
	windowChanged bool
	ptyErr        error
	shellErr      error
	windowErr     error
}

func (s *ptyTestSession) RequestPty(term string, h, w int, modes ssh.TerminalModes) error {
	if s.ptyErr != nil {
		return s.ptyErr
	}
	s.ptyRequested = true
	return nil
}

func (s *ptyTestSession) Shell() error {
	if s.shellErr != nil {
		return s.shellErr
	}
	s.shellStarted = true
	return nil
}

func (s *ptyTestSession) WindowChange(h, w int) error {
	if s.windowErr != nil {
		return s.windowErr
	}
	s.windowChanged = true
	return nil
}

func (s *ptyTestSession) Close() error {
	return nil
}

func TestPTYStart(t *testing.T) {
	tests := []struct {
		name      string
		session   *ptyTestSession
		wantErr   bool
		errString string
	}{
		{
			name:    "success",
			session: &ptyTestSession{},
			wantErr: false,
		},
		{
			name: "pty request fails",
			session: &ptyTestSession{
				ptyErr: errors.New("pty failed"),
			},
			wantErr:   true,
			errString: "PTY request failed: pty failed",
		},
		{
			name: "shell fails",
			session: &ptyTestSession{
				shellErr: errors.New("shell failed"),
			},
			wantErr:   true,
			errString: "shell request failed: shell failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pty := NewPTY(tt.session, "xterm")
			err := pty.Start(80, 24, ssh.TerminalModes{})

			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err.Error() != tt.errString {
				t.Errorf("Start() error = %v, want %v", err, tt.errString)
				return
			}

			if !tt.wantErr {
				if !tt.session.ptyRequested {
					t.Error("PTY was not requested")
				}
				if !tt.session.shellStarted {
					t.Error("Shell was not started")
				}
			}
		})
	}
}

func TestPTYResize(t *testing.T) {
	tests := []struct {
		name        string
		session     *ptyTestSession
		initialSize struct{ w, h int }
		newSize     struct{ w, h int }
		wantChange  bool
		wantErr     bool
	}{
		{
			name:        "no change",
			session:     &ptyTestSession{},
			initialSize: struct{ w, h int }{80, 24},
			newSize:     struct{ w, h int }{80, 24},
			wantChange:  false,
		},
		{
			name:        "size changes",
			session:     &ptyTestSession{},
			initialSize: struct{ w, h int }{80, 24},
			newSize:     struct{ w, h int }{100, 30},
			wantChange:  true,
		},
		{
			name: "resize fails",
			session: &ptyTestSession{
				windowErr: errors.New("resize failed"),
			},
			initialSize: struct{ w, h int }{80, 24},
			newSize:     struct{ w, h int }{100, 30},
			wantChange:  true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pty := NewPTY(tt.session, "xterm")
			pty.width = tt.initialSize.w
			pty.height = tt.initialSize.h

			err := pty.Resize(tt.newSize.w, tt.newSize.h)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.wantChange != tt.session.windowChanged {
				t.Errorf("Resize() windowChanged = %v, want %v",
					tt.session.windowChanged, tt.wantChange)
			}
		})
	}
}
