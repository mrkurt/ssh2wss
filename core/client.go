package core

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flyssh/core/log"

	"github.com/creack/pty"
	"golang.org/x/net/http2"
	"golang.org/x/term"
)

// Client represents a terminal client
type Client struct {
	url       string
	authToken string
	stdin     io.Reader
	stdout    io.Writer
	sessionID string
	client    *http.Client
}

// NewClient creates a new terminal client
func NewClient(url string, authToken string) *Client {
	// Create H2 client with timeouts
	client := &http.Client{
		Timeout: 60 * time.Second, // Overall timeout
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				dialer := &net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 15 * time.Second,
				}
				return dialer.Dial(network, addr)
			},
			ReadIdleTimeout:  60 * time.Second,
			PingTimeout:      15 * time.Second,
			WriteByteTimeout: 15 * time.Second,
		},
	}

	return &Client{
		url:       url,
		authToken: authToken,
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		client:    client,
	}
}

// SetIO sets the input and output streams for the client
func (c *Client) SetIO(stdin io.Reader, stdout io.Writer) {
	c.stdin = stdin
	c.stdout = stdout
}

// Connect establishes an H2 connection and starts the terminal session
func (c *Client) Connect() error {
	// Put terminal in raw mode if it's a real terminal
	if f, ok := c.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		oldState, err := term.MakeRaw(int(f.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set up terminal: %v", err)
		}
		defer term.Restore(int(f.Fd()), oldState)
	}

	// Parse URL and add token
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Convert ws:// to http://
	if u.Scheme == "ws" {
		u.Scheme = "http"
	} else if u.Scheme == "wss" {
		u.Scheme = "https"
	}

	// Add path and token
	u.Path = "/terminal"
	q := u.Query()
	q.Set("token", c.authToken)
	u.RawQuery = q.Encode()

	// Create pipe for stdin
	pr, pw := io.Pipe()

	// Start copying stdin to pipe
	go func() {
		io.Copy(pw, c.stdin)
		pw.Close()
	}()

	// Create request with context for keepalive
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Start keepalive goroutine
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Send a ping by making a HEAD request to /ping
				pingURL := *u
				pingURL.Path = "/ping"
				pingReq, err := http.NewRequestWithContext(ctx, http.MethodHead, pingURL.String(), nil)
				if err != nil {
					log.Debug.Printf("Failed to create ping request: %v", err)
					continue
				}
				resp, err := c.client.Do(pingReq)
				if err != nil {
					log.Debug.Printf("Failed to send ping: %v", err)
					continue
				}
				resp.Body.Close()
			}
		}
	}()

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	// Read session ID
	var msg struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
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

	// Copy response to stdout
	_, err = io.Copy(c.stdout, resp.Body)
	if err != nil && err != io.EOF {
		return fmt.Errorf("IO error: %v", err)
	}

	return nil
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
	if u.Scheme == "ws" {
		u.Scheme = "http"
	} else if u.Scheme == "wss" {
		u.Scheme = "https"
	}
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

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}
