// Package server provides SSH server functionality
package server

import (
	"fmt"
	"log"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
)

// SSHServer handles SSH connections
type SSHServer struct {
	port      int
	sshConfig *ssh.ServerConfig
	logger    *log.Logger
}

// NewSSHServer creates a new SSH server with the given host key
func NewSSHServer(port int, hostKey []byte) (*SSHServer, error) {
	signer, err := ssh.ParsePrivateKey(hostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %v", err)
	}

	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(signer)

	return &SSHServer{
		port:      port,
		sshConfig: config,
		logger:    log.New(os.Stderr, "", log.LstdFlags),
	}, nil
}

// SetLogger sets a custom logger for the SSH server
func (s *SSHServer) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Start starts the SSH server
func (s *SSHServer) Start() error {
	// If port is 0, don't start listening - this server will only handle passed connections
	if s.port == 0 {
		return nil
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", s.port, err)
	}
	defer listener.Close()

	s.logger.Printf("SSH server listening on port %d", s.port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.logger.Printf("Failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a new SSH connection
func (s *SSHServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.logger.Printf("Failed SSH handshake: %v", err)
		return
	}
	defer sshConn.Close()

	s.logger.Printf("New SSH connection from %s", sshConn.RemoteAddr())

	// Handle incoming requests
	go ssh.DiscardRequests(reqs)

	// Service the incoming channel
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			s.logger.Printf("Failed to accept channel: %v", err)
			continue
		}

		// Handle channel requests
		go s.handleChannelRequests(channel, requests)
	}
}
