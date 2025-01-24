//go:build !windows
// +build !windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

type Terminal struct {
	cmd    *exec.Cmd
	pty    *os.File
	width  uint16
	height uint16
}

func NewTerminal(width, height uint16) (*Terminal, error) {
	return &Terminal{
		width:  width,
		height: height,
	}, nil
}

func (t *Terminal) Start(command string) error {
	if command == "" {
		command = getDefaultShell()
	}

	cmd := exec.Command(command)
	t.cmd = cmd

	// Set up process attributes
	setupProcessAttributes(cmd, true)

	// Start with PTY
	pty, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start with PTY: %v", err)
	}
	t.pty = pty

	// Set initial window size
	if err := t.Resize(t.width, t.height); err != nil {
		t.Close()
		return fmt.Errorf("failed to set window size: %v", err)
	}

	return nil
}

func (t *Terminal) Read(p []byte) (int, error) {
	return t.pty.Read(p)
}

func (t *Terminal) Write(p []byte) (int, error) {
	return t.pty.Write(p)
}

func (t *Terminal) Close() error {
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	if t.pty != nil {
		t.pty.Close()
	}
	return nil
}

func (t *Terminal) Resize(width, height uint16) error {
	t.width = width
	t.height = height

	ws := &struct {
		rows    uint16
		cols    uint16
		xpixels uint16
		ypixels uint16
	}{
		rows: height,
		cols: width,
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		t.pty.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func (t *Terminal) WindowSize() (width, height uint16) {
	return t.width, t.height
}
