package core

import (
	"fmt"
	"io"
	"os"

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
	// Connect to WebSocket server
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

	// Put terminal in raw mode if it's a real terminal
	if f, ok := c.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		oldState, err := term.MakeRaw(int(f.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set up terminal: %v", err)
		}
		defer term.Restore(int(f.Fd()), oldState)
	}

	// Forward data in both directions
	errc := make(chan error, 1)

	// stdin -> WebSocket
	go func(ws *websocket.Conn, stdin io.Reader) {
		_, err := io.Copy(ws, stdin)
		errc <- err
	}(ws, c.stdin)

	// WebSocket -> stdout
	go func(stdout io.Writer, ws *websocket.Conn) {
		_, err := io.Copy(stdout, ws)
		errc <- err
	}(c.stdout, ws)

	// Wait for either direction to finish
	if err := <-errc; err != nil && err != io.EOF {
		log.Debug.Printf("IO error %s with %s: %v", c.sessionID, ws.RemoteAddr(), err)
		return fmt.Errorf("IO error: %v", err)
	}
	log.Debug.Printf("Connection closed %s with %s", c.sessionID, ws.RemoteAddr())
	return nil
}
