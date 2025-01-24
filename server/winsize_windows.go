//go:build windows
// +build windows

package server

import (
	"os"
)

func setWinsize(f *os.File, w, h int) error {
	// Windows doesn't support TIOCSWINSZ
	// Terminal resizing is handled by the ConPTY API through github.com/creack/pty
	return nil
}
