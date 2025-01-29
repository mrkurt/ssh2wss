package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"flyssh/auth"

	"golang.org/x/net/websocket"
)

// WebSocketServer handles WebSocket connections with authentication
type WebSocketServer struct {
	port      int
	handler   websocket.Handler
	mux       *http.ServeMux
	sshServer SSHServerHandler
}

// WebSocketWrapper wraps an SSH server with WebSocket transport
type WebSocketWrapper struct {
	sshServer SSHServerHandler
	port      int
	authToken string
}

// SSHServerHandler is the interface that must be implemented by SSH servers
type SSHServerHandler interface {
	// handleConnection handles a new SSH connection
	handleConnection(conn net.Conn)
}

// NewWebSocketServer creates a new WebSocket server
func NewWebSocketServer(port int) *WebSocketServer {
	ws := &WebSocketServer{
		port: port,
		mux:  http.NewServeMux(),
	}
	ws.handler = websocket.Handler(ws.handleConnection)
	return ws
}

// SetSSHServer sets the SSH server handler
func (s *WebSocketServer) SetSSHServer(sshServer SSHServerHandler) {
	s.sshServer = sshServer
}

// NewWebSocketWrapper creates a new WebSocket wrapper around an SSH server
func NewWebSocketWrapper(sshServer SSHServerHandler, port int, authToken string) *WebSocketWrapper {
	return &WebSocketWrapper{
		sshServer: sshServer,
		port:      port,
		authToken: authToken,
	}
}

// NewWebSocketSSHServer creates a new SSH server wrapped with WebSocket support
func NewWebSocketSSHServer(wsPort int, hostKey []byte) (*WebSocketWrapper, error) {
	// Create the SSH server (without a listening port)
	sshServer, err := NewSSHServer(0, hostKey) // Port 0 means it won't listen
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH server: %v", err)
	}

	// Get auth token from environment
	authToken := os.Getenv("WSS_AUTH_TOKEN")
	if authToken == "" {
		return nil, fmt.Errorf("WSS_AUTH_TOKEN environment variable not set")
	}

	// Create the WebSocket wrapper around the SSH server
	wrapper := NewWebSocketWrapper(sshServer, wsPort, authToken)

	return wrapper, nil
}

// Start starts the WebSocket server
func (s *WebSocketServer) Start() error {
	s.mux.Handle("/", s.withAuth(s.handler))
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("WebSocket server listening on port %d", s.port)
	return http.ListenAndServe(addr, s.mux)
}

// handleConnection handles an authenticated WebSocket connection
func (s *WebSocketServer) handleConnection(ws *websocket.Conn) {
	log.Printf("WebSocket connection authenticated")
	// The WebSocket connection implements net.Conn, so we can pass it directly
	s.sshServer.handleConnection(ws)
}

func executeCommand(cmd string) ([]byte, error) {
	// Create command with the system's default shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	command := exec.Command(shell, "-c", cmd)
	return command.CombinedOutput()
}

// withAuth wraps a WebSocket handler with token authentication
func (s *WebSocketServer) withAuth(wsHandler websocket.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the auth token from the request header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Printf("No Authorization header provided")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if it's a Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			log.Printf("Invalid Authorization header format")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Get the expected token from environment
		expectedToken := os.Getenv("WSS_AUTH_TOKEN")
		validator, err := auth.NewTokenValidator(expectedToken)
		if err != nil {
			log.Printf("Token validation setup failed: %v", err)
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		// Verify the token
		if err := validator.ValidateToken(parts[1]); err != nil {
			log.Printf("Token validation failed: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Token is valid, upgrade to WebSocket
		wsHandler.ServeHTTP(w, r)
	})
}

// Start starts the WebSocket server
func (w *WebSocketWrapper) Start() error {
	handler := websocket.Handler(func(ws *websocket.Conn) {
		// Verify auth token
		if !w.verifyToken(ws.Request()) {
			ws.Close()
			return
		}

		// Handle the connection by passing it to the SSH server
		w.sshServer.handleConnection(ws)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", w.port),
		Handler: handler,
	}

	return server.ListenAndServe()
}

// verifyToken verifies the Bearer token in the Authorization header
func (w *WebSocketWrapper) verifyToken(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return token == w.authToken
}
