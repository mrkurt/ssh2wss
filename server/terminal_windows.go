//go:build windows
// +build windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

type Terminal struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
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

	// Create pipes for stdin/stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	t.stdin = pw

	or, ow, err := os.Pipe()
	if err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	t.stdout = or

	// Create the command
	cmd := exec.Command(command)
	t.cmd = cmd

	// Set up process attributes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}

	// Connect pipes
	cmd.Stdin = pr
	cmd.Stdout = ow
	cmd.Stderr = ow

	// Start the process
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		ow.Close()
		or.Close()
		return fmt.Errorf("failed to start process: %v", err)
	}

	// Close the child's ends of the pipes
	pr.Close()
	ow.Close()

	return nil
}

func (t *Terminal) Read(p []byte) (int, error) {
	return t.stdout.Read(p)
}

func (t *Terminal) Write(p []byte) (int, error) {
	return t.stdin.Write(p)
}

func (t *Terminal) Close() error {
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.stdout != nil {
		t.stdout.Close()
	}
	return nil
}

func (t *Terminal) Resize(width, height uint16) error {
	t.width = width
	t.height = height
	// Windows terminal resizing is handled by the ConPTY API
	// We'll implement this later using golang.org/x/sys/windows
	return nil
}

func (t *Terminal) WindowSize() (width, height uint16) {
	return t.width, t.height
}
