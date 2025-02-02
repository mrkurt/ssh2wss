//go:build windows
// +build windows

package core

import (
	"fmt"
	"os"
	"time"

	"flyssh/core/log"

	"golang.org/x/term"
)

// setupWindowSizeHandler sets up window size polling for Windows
func (c *Client) setupWindowSizeHandler() {
	// Get initial size
	if f, ok := c.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fd := int(f.Fd()) // Capture fd for goroutine
		lastWidth, lastHeight, err := term.GetSize(fd)
		if err != nil {
			log.Debug.Printf("Failed to get initial window size: %v", err)
			return
		}

		// Send initial window size
		if err := c.sendWindowSize(); err != nil {
			log.Debug.Printf("Failed to send initial window size: %v", err)
			return
		}

		// Start polling for window size changes
		go func(fd int, client *Client) {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			lastW, lastH := lastWidth, lastHeight // Capture initial sizes

			for range ticker.C {
				width, height, err := term.GetSize(fd)
				if err != nil {
					log.Debug.Printf("Failed to get window size: %v", err)
					return
				}

				if width != lastW || height != lastH {
					if err := client.sendWindowSize(); err != nil {
						log.Debug.Printf("Failed to send window size: %v", err)
						return
					}
					lastW, lastH = width, height
				}
			}
		}(fd, c)
	}
}

// getWindowSize gets the current window size on Windows
func (c *Client) getWindowSize() (rows, cols, xpixels, ypixels uint16, err error) {
	if f, ok := c.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		width, height, err := term.GetSize(int(f.Fd()))
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("failed to get window size: %v", err)
		}
		// Windows doesn't provide pixel dimensions, so we use 0
		return uint16(height), uint16(width), 0, 0, nil
	}
	return 0, 0, 0, 0, fmt.Errorf("not a terminal")
}
