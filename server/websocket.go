package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"ssh2wss/auth"

	"golang.org/x/net/websocket"
)

// WebSocketServer handles WebSocket connections with authentication
type WebSocketServer struct {
	port    int
	handler websocket.Handler
}

// NewWebSocketServer creates a new WebSocket server
func NewWebSocketServer(port int) *WebSocketServer {
	ws := &WebSocketServer{
		port: port,
	}
	ws.handler = websocket.Handler(ws.handleConnection)
	return ws
}

// Start starts the WebSocket server
func (s *WebSocketServer) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.withAuth(s.handler),
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("WebSocket server listening on port %d", s.port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleConnection handles an authenticated WebSocket connection
func (s *WebSocketServer) handleConnection(ws *websocket.Conn) {
	log.Printf("WebSocket connection authenticated")

	// Handle the WebSocket connection
	var buf [1024]byte
	for {
		n, err := ws.Read(buf[:])
		if err != nil {
			if err != io.EOF {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		cmd := string(buf[:n])
		log.Printf("WebSocket received command: %s", cmd)

		// Execute the command and send back the response
		if strings.HasPrefix(cmd, "echo") {
			response := strings.TrimPrefix(cmd, "echo ")
			response = strings.TrimSpace(response)
			response = fmt.Sprintf("%s\n", response)
			if _, err := ws.Write([]byte(response)); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		}
	}
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
