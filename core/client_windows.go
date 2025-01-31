//go:build windows
// +build windows

package core

import (
	"time"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

// setupWindowResize sets up window resize handling for Windows systems
func (c *Client) setupWindowResize(ws *websocket.Conn, fd int) {
	// Get initial size
	lastWidth, lastHeight, err := term.GetSize(fd)
	if err != nil {
		return
	}

	// Send initial window size
	if err := sendWindowSize(ws, lastWidth, lastHeight); err != nil {
		return
	}

	// Start a goroutine to handle window resizing
	go func(ws *websocket.Conn, fd int) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			width, height, err := term.GetSize(fd)
			if err != nil {
				return
			}

			if err := sendWindowSize(ws, width, height); err != nil {
				return
			}
		}
	}(ws, fd)
}
