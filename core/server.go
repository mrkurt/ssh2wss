package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"golang.org/x/net/websocket"
)

// GenerateDevToken generates a random token for development mode
func GenerateDevToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Server represents a WebSocket server that handles PTY connections
type Server struct {
	port int
	mux  *http.ServeMux
}

// NewServer creates a new server instance
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	return &Server{
		port: port,
		mux:  mux,
	}
}

// Start starts the WebSocket server
func (s *Server) Start() error {
	// Set up WebSocket handler with auth wrapper
	wsHandler := websocket.Handler(s.handleConnection)
	s.mux.Handle("/", s.withAuth(wsHandler))

	// Start HTTP server
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Starting WebSocket server on %s", addr)
	// nosemgrep: no-direct-http - Server runs behind TLS-terminating reverse proxy
	return http.ListenAndServe(addr, s.mux)
}

// Stop stops the server
func (s *Server) Stop() {
	// Nothing to do yet, but keeping for future use
}

// withAuth wraps a handler with token authentication
func (s *Server) withAuth(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedToken := os.Getenv("WSS_AUTH_TOKEN")
		if expectedToken == "" {
			log.Printf("WSS_AUTH_TOKEN not set")
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		token := r.URL.Query().Get("token")
		if token == "" {
			log.Printf("Missing token from %s", r.RemoteAddr)
			http.Error(w, "Missing auth token", http.StatusUnauthorized)
			return
		}

		if token != expectedToken {
			log.Printf("Invalid token from %s", r.RemoteAddr)
			http.Error(w, "Invalid auth token", http.StatusUnauthorized)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// handleConnection handles a new WebSocket connection
func (s *Server) handleConnection(ws *websocket.Conn) {
	log.Printf("New connection from %s", ws.RemoteAddr())

	// Start a new shell like an SSH server would:
	// 1. Using /bin/sh as the system shell
	// 2. Setting a minimal, controlled environment
	// 3. Matching standard SSH server behavior
	cmd := exec.Command("/bin/sh") // nosemgrep: no-system-exec
	cmd.Env = []string{
		"TERM=dumb",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"SHELL=/bin/sh",
		"PS1=$ ",
	}

	// Create PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start PTY: %v", err)
		ws.Close()
		return
	}
	defer ptmx.Close()

	// Handle resize messages
	go func(ptmx *os.File, ws *websocket.Conn, cmd *exec.Cmd) {
		for {
			var msg struct {
				Type string `json:"type"`
				Data struct {
					Rows uint16 `json:"rows"`
					Cols uint16 `json:"cols"`
				} `json:"data"`
			}

			if err := websocket.JSON.Receive(ws, &msg); err != nil {
				cmd.Process.Kill()
				return
			}

			if msg.Type == "resize" {
				pty.Setsize(ptmx, &pty.Winsize{
					Rows: msg.Data.Rows,
					Cols: msg.Data.Cols,
				})
			}
		}
	}(ptmx, ws, cmd)

	// Copy WebSocket -> PTY
	go func(ptmx *os.File, ws *websocket.Conn, cmd *exec.Cmd) {
		buf := make([]byte, 32*1024)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				cmd.Process.Kill()
				return
			}
			if _, err := ptmx.Write(buf[:n]); err != nil {
				cmd.Process.Kill()
				return
			}
		}
	}(ptmx, ws, cmd)

	// Copy PTY -> WebSocket
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			return
		}
		if _, err := ws.Write(buf[:n]); err != nil {
			return
		}
	}
}
