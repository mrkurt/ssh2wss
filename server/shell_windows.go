//go:build windows
// +build windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Get the full path to cmd.exe
	cmdPath := findInPath("cmd.exe")
	if cmdPath == "" {
		cmdPath = `C:\Windows\System32\cmd.exe` // Fallback to default location
	}

	// Create the command
	var cmd *exec.Cmd
	if isInternalCommand(command) {
		// For internal commands like dir, echo, etc., use cmd.exe /c
		cmd = exec.Command(cmdPath, "/c", command)
	} else {
		// For external commands, still use cmd.exe to handle pipes and redirections
		cmd = exec.Command(cmdPath, "/c", command)
	}
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
	// Close stdin first to signal EOF to the process
	if s.stdin != nil {
		s.stdin.Close()
		s.stdin = nil
	}

	// Wait for the process to finish naturally
	if s.cmd != nil && s.cmd.Process != nil {
		// Try graceful termination first
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
			// If interrupt fails, try termination
			if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				// If termination fails, force kill as last resort
				s.cmd.Process.Kill()
			}
		}
		// Wait for the process to finish and get its exit status
		s.cmd.Wait()
	}

	// Close stdout
	if s.stdout != nil {
		s.stdout.Close()
		s.stdout = nil
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
	// Check common Windows locations first
	commonPaths := []string{
		`C:\Windows\System32`,
		`C:\Windows`,
		`C:\Windows\System32\WindowsPowerShell\v1.0`,
		`C:\Program Files\PowerShell\7`,
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
func (s *Shell) GetExitCode() int {
	if s.cmd == nil || s.cmd.ProcessState == nil {
		return -1
	}

	// On Windows, ProcessState.ExitCode() is the most reliable way to get the exit code
	return s.cmd.ProcessState.ExitCode()
}

// isInternalCommand checks if a command is a Windows internal command
func isInternalCommand(command string) bool {
	// List of Windows internal commands
	internalCmds := []string{
		"dir", "echo", "cd", "type", "copy", "del", "erase", "md", "mkdir",
		"move", "path", "rd", "rmdir", "ren", "rename", "set", "time", "date",
		"ver", "vol", "prompt", "cls", "color",
	}

	// Extract the command name (before any arguments)
	cmdName := strings.Fields(command)[0]
	cmdName = strings.ToLower(cmdName)

	for _, cmd := range internalCmds {
		if cmdName == cmd {
			return true
		}
	}
	return false
}
