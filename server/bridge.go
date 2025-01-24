package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// Bridge ties together the SSH and WebSocket servers
type Bridge struct {
	sshServer *SSHServer
	wsServer  *WebSocketServer
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewBridge creates a new bridge with the given ports and host key
func NewBridge(sshPort, wsPort int, hostKey []byte) (*Bridge, error) {
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
	}, nil
}

// Start starts both the SSH and WebSocket servers
func (b *Bridge) Start() error {
	// Start WebSocket server in a goroutine
	go func() {
		if err := b.wsServer.Start(b.ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("WebSocket server failed: %v", err)
		}
	}()

	// Start SSH server (blocking)
	return b.sshServer.Start(b.ctx)
}

// Stop gracefully shuts down both servers
func (b *Bridge) Stop() {
	log.Println("Bridge Stop() called, canceling context")
	b.cancel()
	log.Println("Bridge context canceled, waiting for servers to shut down")
	time.Sleep(100 * time.Millisecond) // Give servers time to shut down
	log.Println("Bridge shutdown complete")
}
