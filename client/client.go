package client

import (
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// Client represents an SSH client connection
type Client struct {
	client *ssh.Client
}

// Connect creates a new SSH client connection
func Connect(addr string, config *ssh.ClientConfig) (*Client, error) {
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}
	return &Client{client: client}, nil
}

// Close closes the SSH client connection
func (c *Client) Close() error {
	return c.client.Close()
}

// NewInteractiveSession creates a new interactive SSH session
func (c *Client) NewInteractiveSession() (*InteractiveSession, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	// Request PTY with default size
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 80, 24, modes); err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to request PTY: %v", err)
	}

	return &InteractiveSession{session: session}, nil
}

// InteractiveSession represents an interactive SSH session
type InteractiveSession struct {
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	stderr  io.Reader
}

// Close closes the session
func (s *InteractiveSession) Close() error {
	return s.session.Close()
}

// Start starts an interactive shell
func (s *InteractiveSession) Start() error {
	var err error
	s.stdin, err = s.session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	s.stdout, err = s.session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}

	s.stderr, err = s.session.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %v", err)
	}

	return s.session.Shell()
}

// Wait waits for the remote command to exit
func (s *InteractiveSession) Wait() error {
	return s.session.Wait()
}

// Run executes a command in the session
func (s *InteractiveSession) Run(cmd string) error {
	return s.session.Run(cmd)
}

// Write writes data to the session's stdin
func (s *InteractiveSession) Write(data []byte) (int, error) {
	return s.stdin.Write(data)
}

// Resize changes the size of the terminal window
func (s *InteractiveSession) Resize(width, height int) error {
	return s.session.WindowChange(height, width)
}

// Signal sends a signal to the remote process
func (s *InteractiveSession) Signal(sig ssh.Signal) error {
	return s.session.Signal(sig)
}
