package core

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

// Client represents a terminal client
type Client struct {
	url       string
	authToken string
	stdin     io.Reader
	stdout    io.Writer
}

// NewClient creates a new terminal client
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

// Connect connects to a WebSocket server and starts the terminal session
func (c *Client) Connect() error {
	// Connect to WebSocket server
	origin := "http://localhost"
	url := fmt.Sprintf("%s?token=%s", c.url, c.authToken)
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer ws.Close()

	// Set up terminal if stdin is a real terminal
	var oldState *term.State
	if f, ok := c.stdin.(*os.File); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			oldState, err = term.MakeRaw(fd)
			if err != nil {
				return fmt.Errorf("failed to set up terminal: %v", err)
			}
			defer term.Restore(fd, oldState)

			// Set up window resize handling (platform-specific)
			c.setupWindowResize(ws, fd)
		}
	}

	// Handle bidirectional copying using the standard Go pattern for
	// terminal/network IO. This pattern:
	// 1. Uses separate goroutines for each direction to prevent blocking
	// 2. Returns when either direction fails/closes (using errc channel)
	// 3. Matches the approach used in Go's crypto/ssh and other packages
	errc := make(chan error, 1)

	// Copy stdin to websocket
	go func(ws *websocket.Conn, stdin io.Reader, errc chan<- error) {
		_, err := io.Copy(ws, stdin)
		errc <- err
	}(ws, c.stdin, errc)

	// Copy websocket to stdout
	go func(ws *websocket.Conn, stdout io.Writer, errc chan<- error) {
		_, err := io.Copy(stdout, ws)
		errc <- err
	}(ws, c.stdout, errc)

	// Wait for either direction to finish and return
	// We only care about non-EOF errors
	if err := <-errc; err != nil && err != io.EOF {
		return fmt.Errorf("IO error: %v", err)
	}
	return nil
}
