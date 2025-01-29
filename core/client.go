package core

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

// Client represents a WebSocket client that connects to a PTY server
type Client struct {
	url       string
	authToken string
	stdin     io.Reader
	stdout    io.Writer
}

// NewClient creates a new client instance
func NewClient(url string, authToken string) *Client {
	return &Client{
		url:       url,
		authToken: authToken,
		stdin:     os.Stdin,
		stdout:    os.Stdout,
	}
}

// SetIO sets the input and output streams for the client
func (c *Client) SetIO(stdin io.Reader, stdout io.Writer) {
	c.stdin = stdin
	c.stdout = stdout
}

// Connect establishes a WebSocket connection and starts the PTY session
func (c *Client) Connect() error {
	// Connect to WebSocket server
	origin := "http://localhost"
	url := fmt.Sprintf("%s?token=%s", c.url, c.authToken)
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket server: %v", err)
	}
	defer ws.Close()

	// Set up terminal
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to set up terminal: %v", err)
	}
	defer term.Restore(fd, oldState)

	// Handle window resize
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	go func() {
		for range sigwinch {
			if width, height, err := term.GetSize(fd); err == nil {
				msg := struct {
					Type string `json:"type"`
					Data struct {
						Rows uint16 `json:"rows"`
						Cols uint16 `json:"cols"`
					} `json:"data"`
				}{
					Type: "resize",
					Data: struct {
						Rows uint16 `json:"rows"`
						Cols uint16 `json:"cols"`
					}{
						Rows: uint16(height),
						Cols: uint16(width),
					},
				}
				websocket.JSON.Send(ws, msg)
			}
		}
	}()

	// Send initial window size
	if width, height, err := term.GetSize(fd); err == nil {
		msg := struct {
			Type string `json:"type"`
			Data struct {
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			} `json:"data"`
		}{
			Type: "resize",
			Data: struct {
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			}{
				Rows: uint16(height),
				Cols: uint16(width),
			},
		}
		websocket.JSON.Send(ws, msg)
	}

	// Copy input to WebSocket
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := c.stdin.Read(buf)
			if err != nil {
				return
			}
			log.Printf("Client sending to server: %q", buf[:n])
			if _, err := ws.Write(buf[:n]); err != nil {
				return
			}
		}
	}()

	// Copy WebSocket output to stdout
	buf := make([]byte, 32*1024)
	for {
		n, err := ws.Read(buf)
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("failed to read from server: %v", err)
			}
			return nil
		}
		if _, err := c.stdout.Write(buf[:n]); err != nil {
			return fmt.Errorf("failed to write to stdout: %v", err)
		}
	}
}
