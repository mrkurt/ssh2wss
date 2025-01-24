//go:build windows
// +build windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type Shell struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
	width  uint16
	height uint16
}

func NewShell(width, height uint16) (*Shell, error) {
	return &Shell{
		width:  width,
		height: height,
	}, nil
}

func (s *Shell) Start(command string) error {
	if command == "" {
		command = getDefaultShell()
	}

	// Create pipes for stdin/stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	s.stdin = pw

	or, ow, err := os.Pipe()
	if err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	s.stdout = or

	// Create the command
	cmd := exec.Command(command)
	s.cmd = cmd

	// Set up process attributes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
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

func (s *Shell) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *Shell) Write(p []byte) (int, error) {
	return s.stdin.Write(p)
}

func (s *Shell) Close() error {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}
	return nil
}

func (s *Shell) Resize(width, height uint16) error {
	s.width = width
	s.height = height
	return nil
}

func (s *Shell) WindowSize() (width, height uint16) {
	return s.width, s.height
}

func getDefaultShell() string {
	// Use cmd.exe as the default shell on Windows
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return comspec
	}
	return "cmd.exe"
}

func getShellArgs(isLogin bool) []string {
	if isLogin {
		return []string{"/c"}
	}
	return []string{"/c"}
}

func getCommandArgs(command string) []string {
	// On Windows, wrap the command in cmd.exe /c to handle built-in commands
	return []string{"/c", command}
}

// findInPath looks for an executable in PATH
func findInPath(exe string) string {
	// Check common locations first
	commonPaths := []string{
		`C:\Program Files\PowerShell\7`,
		`C:\Windows\System32\WindowsPowerShell\v1.0`,
		`C:\Windows\System32`,
	}

	for _, dir := range commonPaths {
		path := filepath.Join(dir, exe)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Then check PATH
	if path, err := exec.LookPath(exe); err == nil {
		return path
	}

	return ""
}

// GetExitCode returns the process exit code
func (s *Shell) GetExitCode() (uint32, error) {
	if s.cmd == nil || s.cmd.ProcessState == nil {
		return 0, fmt.Errorf("process not started or not finished")
	}
	return uint32(s.cmd.ProcessState.ExitCode()), nil
}
