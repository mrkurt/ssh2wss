package client

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

// Client represents a WebSocket client that connects to a terminal server
type Client struct {
	url       string
	authToken string
	stdin     io.Reader
	stdout    io.Writer
	oldState  *term.State
}

// Message represents a WebSocket message
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WindowSize represents a terminal window size message
type WindowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// New creates a new Client instance
func New(url, authToken string) *Client {
	return &Client{
		url:       url,
		authToken: authToken,
		stdin:     os.Stdin,
		stdout:    os.Stdout,
	}
}

// SetIO sets custom IO readers/writers for testing
func (c *Client) SetIO(stdin io.Reader, stdout io.Writer) {
	c.stdin = stdin
	c.stdout = stdout
}

// Connect establishes a connection to the server and starts a terminal session
func (c *Client) Connect() error {
	// Parse URL
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Add auth token to URL
	q := u.Query()
	q.Set("token", c.authToken)
	u.RawQuery = q.Encode()

	// Set up raw terminal mode if we're on a terminal
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		var err error
		// Save the old state to restore later
		c.oldState, err = term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to set raw terminal mode: %v", err)
		}
		// Ensure we restore the terminal state on exit
		defer func() {
			if err := term.Restore(fd, c.oldState); err != nil {
				log.Printf("Failed to restore terminal: %v", err)
			}
		}()

		// Handle signals for clean terminal restore
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
		go func() {
			<-sigChan
			if err := term.Restore(fd, c.oldState); err != nil {
				log.Printf("Failed to restore terminal on signal: %v", err)
			}
			os.Exit(1)
		}()
	}

	// Connect to WebSocket server
	ws, err := websocket.Dial(u.String(), "", "http://localhost")
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Set up window size handling if we're on a terminal
	if term.IsTerminal(fd) {
		go c.handleWindowChanges(ws)
	}

	// Copy input/output with error handling
	errs := make(chan error, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := c.stdin.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Failed to read from stdin: %v", err)
				}
				errs <- err
				return
			}
			log.Printf("Client sending to server: %q", buf[:n])
			if err := websocket.Message.Send(ws, buf[:n]); err != nil {
				log.Printf("Failed to send to server: %v", err)
				errs <- err
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Failed to read from server: %v", err)
				}
				errs <- err
				return
			}
			log.Printf("Client received from server: %q", buf[:n])
			if _, err := c.stdout.Write(buf[:n]); err != nil {
				log.Printf("Failed to write to stdout: %v", err)
				errs <- err
				return
			}
		}
	}()

	// Wait for either copy to finish
	err = <-errs
	if err != nil && err != io.EOF {
		return fmt.Errorf("I/O error: %v", err)
	}
	return nil
}

// handleWindowChanges sends window size updates to the server
func (c *Client) handleWindowChanges(ws *websocket.Conn) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// Send initial size
	if err := c.sendWindowSize(ws); err != nil {
		log.Printf("Failed to send initial window size: %v", err)
		return
	}

	// Handle window size changes
	for range sigChan {
		if err := c.sendWindowSize(ws); err != nil {
			log.Printf("Failed to send window size: %v", err)
			return
		}
	}
}

// sendWindowSize sends the current terminal size to the server
func (c *Client) sendWindowSize(ws *websocket.Conn) error {
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %v", err)
	}

	msg := Message{
		Type: "resize",
		Data: WindowSize{
			Rows: uint16(height),
			Cols: uint16(width),
		},
	}

	return websocket.JSON.Send(ws, msg)
}
