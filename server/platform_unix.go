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
func setWinsize(f *os.File, w, h uint32) error {
	ws := &struct {
		Height uint16
		Width  uint16
		x      uint16 // unused
		y      uint16 // unused
	}{
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
	return 1, false
}

// setupProcessAttributes configures platform-specific process attributes
func setupProcessAttributes(cmd *exec.Cmd, isPty bool) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Only set Setsid for PTY
	if isPty {
		cmd.SysProcAttr.Setsid = true
	}
}
