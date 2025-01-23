package server

import (
	"fmt"
	"log"
)

// Bridge ties together the SSH and WebSocket servers
type Bridge struct {
	sshServer *SSHServer
	wsServer  *WebSocketServer
}

// NewBridge creates a new bridge with the given ports and host key
func NewBridge(sshPort, wsPort int, hostKey []byte) (*Bridge, error) {
	sshServer, err := NewSSHServer(sshPort, hostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH server: %v", err)
	}

	wsServer := NewWebSocketServer(wsPort)

	return &Bridge{
		sshServer: sshServer,
		wsServer:  wsServer,
	}, nil
}

// Start starts both the SSH and WebSocket servers
func (b *Bridge) Start() error {
	// Start WebSocket server in a goroutine
	go func() {
		if err := b.wsServer.Start(); err != nil {
			log.Printf("WebSocket server failed: %v", err)
		}
	}()

	// Start SSH server (blocking)
	return b.sshServer.Start()
}
