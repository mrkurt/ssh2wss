package server

import (
	"fmt"
	"log"
)

// Bridge connects WebSocket and SSH servers
type Bridge struct {
	wsServer  *WebSocketServer
	sshServer *SSHServer
}

// NewBridge creates a new bridge between WebSocket and SSH servers
func NewBridge(sshServer *SSHServer, wsAddr string) (*Bridge, error) {
	wsServer, err := NewWebSocketServer(sshServer, wsAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create WebSocket server: %w", err)
	}

	return &Bridge{
		wsServer:  wsServer,
		sshServer: sshServer,
	}, nil
}

// Start starts both servers
func (b *Bridge) Start() error {
	// Start SSH server in a goroutine
	go func() {
		if err := b.sshServer.Start(); err != nil {
			log.Printf("SSH server error: %v", err)
		}
	}()

	// Start WebSocket server in the main goroutine
	return b.wsServer.Start()
}
