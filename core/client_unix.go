//go:build unix
// +build unix

package core

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

// setupWindowResize sets up window resize handling for Unix systems
func (c *Client) setupWindowResize(ws *websocket.Conn, fd int) {
	// Handle window resize
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	go func() {
		for range sigwinch {
			if width, height, err := term.GetSize(fd); err == nil {
				if err := sendWindowSize(ws, width, height); err != nil {
					return
				}
			}
		}
	}()

	// Send initial window size
	if width, height, err := term.GetSize(fd); err == nil {
		sendWindowSize(ws, width, height)
	}
}
