package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"log"

	"golang.org/x/crypto/ssh"
)

// generateHostKey generates an in-memory RSA key for the SSH server
func generateHostKey() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %v", err)
	}

	return signer, nil
}

// Bridge ties together the SSH and WebSocket servers
type Bridge struct {
	sshServer *SSHServer
	wsServer  *WebSocketServer
	ctx       context.Context
	cancel    context.CancelFunc
	sshDone   chan struct{} // signals SSH server shutdown complete
	wsDone    chan struct{} // signals WebSocket server shutdown complete
}

// NewBridge creates a new bridge with the given ports
func NewBridge(sshPort, wsPort int) (*Bridge, error) {
	hostKey, err := generateHostKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate host key: %v", err)
	}

	sshServer, err := NewSSHServer(sshPort, hostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH server: %v", err)
	}

	wsServer := NewWebSocketServer(wsPort)
	ctx, cancel := context.WithCancel(context.Background())

	return &Bridge{
		sshServer: sshServer,
		wsServer:  wsServer,
		ctx:       ctx,
		cancel:    cancel,
		sshDone:   make(chan struct{}),
		wsDone:    make(chan struct{}),
	}, nil
}

// Start starts both the SSH and WebSocket servers
func (b *Bridge) Start() error {
	// Start WebSocket server in a goroutine
	go func() {
		if err := b.wsServer.Start(b.ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("WebSocket server failed: %v", err)
		}
		close(b.wsDone)
	}()

	// Start SSH server in a goroutine and capture its error
	go func() {
		if err := b.sshServer.Start(b.ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("SSH server failed: %v", err)
		}
		close(b.sshDone)
	}()

	// Wait for context cancellation
	<-b.ctx.Done()
	return nil
}

// Stop gracefully shuts down both servers
func (b *Bridge) Stop() {
	log.Println("Bridge Stop() called, canceling context")
	b.cancel()
	log.Println("Bridge context canceled, waiting for servers to shut down")

	// Wait for WebSocket server shutdown
	<-b.wsDone
	log.Println("WebSocket server shutdown complete")

	// Wait for SSH server shutdown
	<-b.sshDone
	log.Println("SSH server shutdown complete")

	log.Println("Bridge shutdown complete")
}
