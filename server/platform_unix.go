//go:build !windows
// +build !windows

package server

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

type winsize struct {
	Height uint16
	Width  uint16
	xpixel uint16
	ypixel uint16
}

// setWinsize sets the size of the terminal window
func setWinsize(f *os.File, w, h int) error {
	ws := &winsize{
		Width:  uint16(w),
		Height: uint16(h),
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// getExitStatus extracts the exit code from an ExitError
func getExitStatus(err *exec.ExitError) (uint32, bool) {
	if status, ok := err.Sys().(syscall.WaitStatus); ok {
		return uint32(status.ExitStatus()), true
	}
	return 0, false
}

// setupProcessAttributes configures platform-specific process attributes
func setupProcessAttributes(cmd *exec.Cmd, isPty bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Unix processes don't need special flags for PTY
	}
}
