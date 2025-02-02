package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"flyssh/core/log"

	"github.com/creack/pty"
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
	ws        *websocket.Conn
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

// Connect establishes a WebSocket connection and starts the terminal session
func (c *Client) Connect() error {
	// Parse URL and add token
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	q := u.Query()
	q.Set("token", c.authToken)
	u.RawQuery = q.Encode()

	// Connect to WebSocket server
	origin := "http://" + u.Host
	ws, err := websocket.Dial(u.String(), "", origin)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	c.ws = ws

	// Get session ID from server
	var msg struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
	}
	if err := websocket.JSON.Receive(ws, &msg); err != nil {
		return fmt.Errorf("failed to get session ID: %v", err)
	}
	if msg.Type != "session" {
		return fmt.Errorf("unexpected message type: %s", msg.Type)
	}
	c.sessionID = msg.SessionID

	// Handle window size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go c.handleWindowChanges(ch)

	// Send initial window size
	if err := c.sendWindowSize(); err != nil {
		log.Debug.Printf("Failed to send initial window size: %v", err)
	}

	// Start terminal session
	return c.startSession()
}

// handleWindowChanges handles window resize signals
func (c *Client) handleWindowChanges(ch chan os.Signal) {
	for range ch {
		if err := c.sendWindowSize(); err != nil {
			log.Debug.Printf("Failed to send window size: %v", err)
		}
	}
}

// sendWindowSize sends the current window size to the server
func (c *Client) sendWindowSize() error {
	ws, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to get window size: %v", err)
	}

	// Construct URL for window size endpoint
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	u.Scheme = "http"
	u.Path = fmt.Sprintf("/session/%s/winsize", c.sessionID)
	q := u.Query()
	q.Set("token", c.authToken)
	u.RawQuery = q.Encode()

	// Send request
	body := struct {
		Rows    uint16 `json:"rows"`
		Cols    uint16 `json:"cols"`
		XPixels uint16 `json:"x_pixels"`
		YPixels uint16 `json:"y_pixels"`
	}{
		Rows:    ws.Rows,
		Cols:    ws.Cols,
		XPixels: ws.X,
		YPixels: ws.Y,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal window size: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// startSession starts the terminal session
func (c *Client) startSession() error {
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
	}(c.ws, c.stdin)

	// WebSocket -> stdout
	go func(stdout io.Writer, ws *websocket.Conn) {
		_, err := io.Copy(stdout, ws)
		errc <- err
	}(c.stdout, c.ws)

	// Wait for either direction to finish
	if err := <-errc; err != nil && err != io.EOF {
		log.Debug.Printf("IO error %s with %s: %v", c.sessionID, c.ws.RemoteAddr(), err)
		return fmt.Errorf("IO error: %v", err)
	}
	log.Debug.Printf("Connection closed %s with %s", c.sessionID, c.ws.RemoteAddr())
	return nil
}
