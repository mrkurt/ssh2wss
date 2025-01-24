//go:build windows
// +build windows

package server

import (
	"os"
	"os/exec"
	"syscall"
)

type winsize struct {
	Height uint16
	Width  uint16
	xpixel uint16
	ypixel uint16
}

// setWinsize sets the size of the terminal window
// On Windows without ConPTY, this is a no-op
func setWinsize(f *os.File, w, h int) error {
	// We'll implement proper window resizing when we add ConPTY support
	return nil
}

// getExitStatus extracts the exit code from an ExitError
func getExitStatus(err *exec.ExitError) (uint32, bool) {
	if status, ok := err.Sys().(syscall.WaitStatus); ok {
		return uint32(status.ExitCode), true
	}
	return 0, false
}

// setupProcessAttributes configures platform-specific process attributes
func setupProcessAttributes(cmd *exec.Cmd, isPty bool) {
	if isPty {
		// PTY mode - no special flags needed
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	} else {
		// Non-PTY mode - create new process group
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
	}
}
