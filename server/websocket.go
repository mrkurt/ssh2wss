package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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
	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", s.port),
		Handler: s.withAuth(s.handler),
	}

	// Create a channel to signal shutdown completion
	shutdownComplete := make(chan struct{})

	// Start server in a goroutine
	go func() {
		defer close(shutdownComplete)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("WebSocket server context canceled, shutting down")

	// Create a timeout context for shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown the server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("WebSocket server shutdown error: %v", err)
		return err
	}

	// Wait for server to finish
	<-shutdownComplete
	return nil
}

// handleConnection handles an authenticated WebSocket connection
func (s *WebSocketServer) handleConnection(ws *websocket.Conn) {
	log.Printf("WebSocket connection authenticated")

	// Handle the WebSocket connection
	for {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			if err == io.EOF {
				log.Printf("WebSocket connection closed by client")
			} else if _, ok := err.(*websocket.ProtocolError); ok {
				log.Printf("WebSocket protocol error (malformed frame): %v", err)
			} else {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		log.Printf("WebSocket received command: %s", msg)

		// Execute the command and send back the response
		if strings.HasPrefix(msg, "echo") {
			response := strings.TrimPrefix(msg, "echo ")
			response = strings.TrimSpace(response)
			response = fmt.Sprintf("%s\n", response)
			if err := websocket.Message.Send(ws, response); err != nil {
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
