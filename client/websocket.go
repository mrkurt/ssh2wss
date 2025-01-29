package client

import (
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

// WebSocketClient represents a WebSocket-based SSH client
type WebSocketClient struct {
	wsURL     string
	authToken string
	config    *ssh.ClientConfig
}

// NewWebSocketClient creates a new WebSocket-based SSH client
func NewWebSocketClient(wsURL, authToken string, config *ssh.ClientConfig) *WebSocketClient {
	return &WebSocketClient{
		wsURL:     wsURL,
		authToken: authToken,
		config:    config,
	}
}

// Connect establishes a WebSocket connection to the SSH server
func (c *WebSocketClient) Connect() (*Client, error) {
	// Parse WebSocket URL
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid WebSocket URL: %v", err)
	}

	// Create origin URL (required for WebSocket handshake)
	origin := &url.URL{
		Scheme: "http",
		Host:   u.Host,
	}

	// Create WebSocket config
	wsConfig := &websocket.Config{
		Location: u,
		Origin:   origin,
		Header: http.Header{
			"Authorization": []string{"Bearer " + c.authToken},
		},
		Protocol: []string{"ssh"},
		Version:  websocket.ProtocolVersionHybi13,
	}

	// Connect to WebSocket server
	ws, err := websocket.DialConfig(wsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket server: %v", err)
	}

	// Create SSH client connection over WebSocket
	conn, chans, reqs, err := ssh.NewClientConn(ws, u.Host, c.config)
	if err != nil {
		ws.Close()
		return nil, fmt.Errorf("failed to create SSH client connection: %v", err)
	}

	// Create SSH client
	sshClient := ssh.NewClient(conn, chans, reqs)
	return &Client{client: sshClient}, nil
}
