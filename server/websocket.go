package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"flyssh/auth"

	"golang.org/x/net/websocket"
)

// WebSocketServer handles WebSocket connections with authentication
type WebSocketServer struct {
	port      int
	handler   websocket.Handler
	mux       *http.ServeMux
	sshServer SSHServerHandler
	upgrader  *websocket.Server
	server    *http.Server
}

// WebSocketWrapper wraps a WebSocket connection to implement net.Conn
type WebSocketWrapper struct {
	conn      *websocket.Conn
	sshServer SSHServerHandler
	readBuf   bytes.Buffer
	writeBuf  bytes.Buffer
	closed    bool
	readLock  sync.Mutex
	writeLock sync.Mutex
}

// SSHServerHandler defines the interface for SSH server implementations
type SSHServerHandler interface {
	HandleConnection(net.Conn) error
}

// NewWebSocketServer creates a new WebSocket server
func NewWebSocketServer(sshServer SSHServerHandler, addr string) (*WebSocketServer, error) {
	ws := &WebSocketServer{
		sshServer: sshServer,
	}

	// Create WebSocket handler
	handler := websocket.Handler(ws.handleConnection)

	// Create HTTP server
	ws.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	return ws, nil
}

// SetSSHServer sets the SSH server handler
func (s *WebSocketServer) SetSSHServer(sshServer SSHServerHandler) {
	s.sshServer = sshServer
}

// NewWebSocketWrapper creates a new WebSocket wrapper
func NewWebSocketWrapper(conn *websocket.Conn) *WebSocketWrapper {
	return &WebSocketWrapper{
		conn: conn,
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
	wrapper := NewWebSocketWrapper(nil)
	wrapper.sshServer = sshServer

	return wrapper, nil
}

// Start starts the WebSocket server
func (s *WebSocketServer) Start() error {
	s.mux.Handle("/", s.withAuth(s.handler))
	return s.server.ListenAndServe()
}

// handleConnection handles a new WebSocket connection
func (s *WebSocketServer) handleConnection(ws *websocket.Conn) {
	log.Printf("New WebSocket connection from %s", ws.RemoteAddr())

	// Create a WebSocket wrapper that implements net.Conn
	wrapper := NewWebSocketWrapper(ws)
	defer wrapper.Close()

	// Handle the connection with the SSH server
	if err := s.sshServer.HandleConnection(wrapper); err != nil {
		log.Printf("SSH connection error: %v", err)
	}
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

// SetSSHServer sets the SSH server for this wrapper
func (w *WebSocketWrapper) SetSSHServer(sshServer SSHServerHandler) {
	w.sshServer = sshServer
}

// handleSSHConnection handles the SSH connection over WebSocket
func (w *WebSocketWrapper) handleSSHConnection() {
	if err := w.sshServer.HandleConnection(w); err != nil {
		log.Printf("SSH connection error: %v", err)
	}
}

// Read implements net.Conn
func (w *WebSocketWrapper) Read(b []byte) (n int, err error) {
	w.readLock.Lock()
	defer w.readLock.Unlock()

	if w.closed {
		return 0, io.EOF
	}

	// If buffer is empty, read from WebSocket
	if w.readBuf.Len() == 0 {
		var buf []byte
		if err := websocket.Message.Receive(w.conn, &buf); err != nil {
			if err == io.EOF {
				w.closed = true
			}
			return 0, err
		}
		w.readBuf.Write(buf)
	}

	return w.readBuf.Read(b)
}

// Write implements net.Conn
func (w *WebSocketWrapper) Write(b []byte) (n int, err error) {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()

	if w.closed {
		return 0, fmt.Errorf("connection closed")
	}

	if err := websocket.Message.Send(w.conn, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

// Close implements net.Conn
func (w *WebSocketWrapper) Close() error {
	if !w.closed {
		w.closed = true
		return w.conn.Close()
	}
	return nil
}

// LocalAddr implements net.Conn
func (w *WebSocketWrapper) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

// RemoteAddr implements net.Conn
func (w *WebSocketWrapper) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

// SetDeadline implements net.Conn
func (w *WebSocketWrapper) SetDeadline(t time.Time) error {
	return w.conn.SetDeadline(t)
}

// SetReadDeadline implements net.Conn
func (w *WebSocketWrapper) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (w *WebSocketWrapper) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}
