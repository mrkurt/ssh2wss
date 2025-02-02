//go:build !windows
// +build !windows

package core

import (
	"os"
	"os/signal"
	"syscall"

	"flyssh/core/log"

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

// setupWindowSizeHandler sets up window size change handling for Unix systems
func (c *Client) setupWindowSizeHandler() {
	// Handle window size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go c.handleWindowChanges(ch)

	// Send initial window size
	if err := c.sendWindowSize(); err != nil {
		log.Debug.Printf("Failed to send initial window size: %v", err)
	}
}
