package core

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"flyssh/core/log"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

// Client represents a terminal client
type Client struct {
	url       string
	authToken string
	stdin     io.Reader
	stdout    io.Writer
	sessionID string
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
	// Connect to WebSocket server for data
	origin := "http://localhost"
	url := fmt.Sprintf("%s?token=%s", c.url, c.authToken)
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer ws.Close()

	log.Debug.Printf("Connected to server at %s", ws.RemoteAddr())

	// Wait for session ID
	var msg struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
	}
	if err := websocket.JSON.Receive(ws, &msg); err != nil {
		return fmt.Errorf("failed to receive session ID: %v", err)
	}
	if msg.Type != "session" {
		return fmt.Errorf("expected session message, got %s", msg.Type)
	}
	c.sessionID = msg.SessionID

	log.Debug.Printf("Session established %s with %s", c.sessionID, ws.RemoteAddr())

	// Connect control channel
	controlURL := fmt.Sprintf("%s/control?token=%s", c.url, c.authToken)
	control, err := websocket.Dial(controlURL, "", origin)
	if err != nil {
		return fmt.Errorf("failed to connect control channel: %v", err)
	}
	defer control.Close()

	log.Debug.Printf("Control channel connected %s to %s", c.sessionID, control.RemoteAddr())

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

			// Set up window resize handling
			go c.handleResize(control, fd)
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
		log.Debug.Printf("Starting stdin->websocket copy %s to %s", c.sessionID, ws.RemoteAddr())
		_, err := io.Copy(ws, stdin)
		if err != nil && err != io.EOF {
			log.Debug.Printf("stdin->websocket error %s to %s: %v", c.sessionID, ws.RemoteAddr(), err)
		}
		errc <- err
	}(ws, c.stdin, errc)

	// Copy websocket to stdout
	go func(ws *websocket.Conn, stdout io.Writer, errc chan<- error) {
		log.Debug.Printf("Starting websocket->stdout copy %s from %s", c.sessionID, ws.RemoteAddr())
		_, err := io.Copy(stdout, ws)
		if err != nil && err != io.EOF {
			log.Debug.Printf("websocket->stdout error %s from %s: %v", c.sessionID, ws.RemoteAddr(), err)
		}
		errc <- err
	}(ws, c.stdout, errc)

	// Wait for either direction to finish and return
	// We only care about non-EOF errors
	if err := <-errc; err != nil && err != io.EOF {
		log.Debug.Printf("IO error %s with %s: %v", c.sessionID, ws.RemoteAddr(), err)
		return fmt.Errorf("IO error: %v", err)
	}
	log.Debug.Printf("Connection closed %s with %s", c.sessionID, ws.RemoteAddr())
	return nil
}

// handleResize handles window resize events
func (c *Client) handleResize(ws *websocket.Conn, fd int) {
	// Send initial window size
	if width, height, err := term.GetSize(fd); err == nil {
		msg := struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Data      struct {
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			} `json:"data"`
		}{
			Type:      "resize",
			SessionID: c.sessionID,
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

	// Handle window resize
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	for range sigwinch {
		if width, height, err := term.GetSize(fd); err == nil {
			msg := struct {
				Type      string `json:"type"`
				SessionID string `json:"session_id"`
				Data      struct {
					Rows uint16 `json:"rows"`
					Cols uint16 `json:"cols"`
				} `json:"data"`
			}{
				Type:      "resize",
				SessionID: c.sessionID,
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
}
