//go:build !windows
// +build !windows

package server

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	rows    uint16
	cols    uint16
	xpixels uint16
	ypixels uint16
}

func setWinsize(f *os.File, w, h int) error {
	ws := &winsize{
		rows: uint16(h),
		cols: uint16(w),
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
