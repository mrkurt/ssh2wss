package client

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"golang.org/x/term"
)

type ClientConfig struct {
	WSServer    string
	AuthToken   string
	User        string
	Destination string
	Port        int
	Identity    string
	Command     string // Command to execute in non-interactive mode
	IsProxy     bool   // Whether this is running in proxy mode
}

type Client struct {
	config *ClientConfig
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Terminal represents a terminal state
type Terminal struct {
	oldState *term.State
	fd       int
}

// Restore restores the terminal to its original state
func (t *Terminal) Restore() error {
	if t.oldState != nil {
		return term.Restore(t.fd, t.oldState)
	}
	return nil
}

func NewClient(config *ClientConfig) (*Client, error) {
	return &Client{
		config: config,
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}, nil
}

func (c *Client) connectWebSocket() (*websocket.Conn, error) {
	wsURL, err := url.Parse(c.config.WSServer)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	config := &websocket.Config{
		Location: wsURL,
		Origin:   &url.URL{Scheme: wsURL.Scheme, Host: wsURL.Host},
		Version:  websocket.ProtocolVersionHybi13,
		Header: map[string][]string{
			"Authorization": {fmt.Sprintf("Bearer %s", c.config.AuthToken)},
		},
	}

	return websocket.DialConfig(config)
}

func (c *Client) Run() error {
	if c.config.IsProxy {
		return c.runProxy()
	}
	return c.runSSH()
}

// runProxy handles proxy mode - pure WebSocket relay for external SSH clients
func (c *Client) runProxy() error {
	// Connect to WebSocket server
	ws, err := c.connectWebSocket()
	if err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}
	defer ws.Close()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Start I/O handling
	errChan := make(chan error, 1)
	go func() {
		// Copy stdin to WebSocket
		_, err := io.Copy(ws, c.stdin)
		if err != nil {
			errChan <- fmt.Errorf("stdin copy failed: %w", err)
		}
	}()

	go func() {
		// Copy WebSocket to stdout
		_, err := io.Copy(c.stdout, ws)
		if err != nil {
			errChan <- fmt.Errorf("stdout copy failed: %w", err)
		}
	}()

	// Wait for completion or error
	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		return fmt.Errorf("received signal: %v", sig)
	}
}

// execRequest represents an SSH exec request
type execRequest struct {
	Command string
}

// exitStatus represents an SSH exit-status request
type exitStatus struct {
	Status uint32
}

// wsNetConn wraps a websocket.Conn to implement net.Conn interface
type wsNetConn struct {
	*websocket.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
}

func newWSNetConn(ws *websocket.Conn) *wsNetConn {
	// Create fake addresses since WebSocket doesn't have the same addressing as TCP
	localAddr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
	remoteAddr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2222}

	return &wsNetConn{
		Conn:       ws,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
}

// Required net.Conn interface methods
func (w *wsNetConn) LocalAddr() net.Addr  { return w.localAddr }
func (w *wsNetConn) RemoteAddr() net.Addr { return w.remoteAddr }

func (w *wsNetConn) SetDeadline(t time.Time) error      { return nil }
func (w *wsNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (w *wsNetConn) SetWriteDeadline(t time.Time) error { return nil }

// runSSH handles direct client mode using SSH protocol
//
// IMPORTANT: This uses an intentionally simple protocol where raw bytes are passed directly
// through the WebSocket connection. DO NOT modify this to add JSON, framing, or any other protocol.
// The SSH functionality is handled entirely by the SSH server on one end and SSH client libraries
// on the other - the WebSocket is just a dumb pipe between them.
//
// The WebSocket connection acts as a pure bidirectional byte stream, exactly like a TCP connection
// would, allowing the SSH protocol to work unchanged over WebSocket transport.
func (c *Client) runSSH() error {
	log.Printf("Starting SSH-over-WebSocket connection")

	// Connect to WebSocket server
	ws, err := c.connectWebSocket()
	if err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}
	defer ws.Close()
	log.Printf("WebSocket connection established")

	// Create net.Conn wrapper for WebSocket
	conn := newWSNetConn(ws)
	log.Printf("Created net.Conn wrapper for WebSocket")

	// Configure SSH client
	config := &ssh.ClientConfig{
		User:            "user", // Doesn't matter since server accepts all
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Create SSH client over WebSocket transport
	log.Printf("Starting SSH handshake over WebSocket")
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", config)
	if err != nil {
		return fmt.Errorf("ssh handshake failed: %w", err)
	}
	defer sshConn.Close()
	log.Printf("SSH handshake completed successfully")

	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	// Create SSH session
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()
	log.Printf("SSH session created")

	// Connect session to stdio
	session.Stdout = c.stdout
	session.Stderr = c.stderr
	session.Stdin = c.stdin

	if c.config.Command != "" {
		log.Printf("Running command over SSH: %s", c.config.Command)
		return session.Run(c.config.Command)
	}

	// Interactive mode
	term, err := c.setupTerminal()
	if err != nil {
		return fmt.Errorf("terminal setup failed: %w", err)
	}
	defer term.Restore()

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return fmt.Errorf("request for PTY failed: %w", err)
	}

	// Start shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// Wait for session to finish
	return session.Wait()
}

func (c *Client) setupTerminal() (*Terminal, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("stdin is not a terminal")
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}

	return &Terminal{oldState: oldState, fd: fd}, nil
}

// wsConn wraps a websocket connection to look like an SSH channel
type wsConn struct {
	*websocket.Conn
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (w *wsConn) Read(p []byte) (n int, err error) {
	return w.Conn.Read(p)
}

func (w *wsConn) Write(p []byte) (n int, err error) {
	return w.Conn.Write(p)
}

func (c *Client) handleSSHIO(channel *wsConn) error {
	// Copy data between the WebSocket and stdio
	go func() {
		io.Copy(channel, channel.stdin)
	}()
	io.Copy(channel.stdout, channel)

	return nil
}
