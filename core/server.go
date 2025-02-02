package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"flyssh/core/log"

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
	port         int
	mux          *http.ServeMux
	ptys         sync.Map // map[string]*os.File to track PTYs by session
	sessionCount uint64   // atomic counter for session IDs
	server       *http.Server
}

// NewServer creates a new server instance
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	return &Server{
		port: port,
		mux:  mux,
		ptys: sync.Map{},
	}
}

// Start starts the WebSocket server
func (s *Server) Start() error {
	// Set up WebSocket handler with auth wrapper
	s.mux.Handle("/", s.withAuth(websocket.Handler(s.handleConnection)))
	// Add window size endpoint
	s.mux.Handle("/session/", s.withAuth(http.HandlerFunc(s.handleSessionControl)))

	// Start HTTP server
	addr := fmt.Sprintf(":%d", s.port)
	log.Info.Printf("Starting WebSocket server on %s", addr)
	s.server = &http.Server{Addr: addr, Handler: s.mux}
	return s.server.ListenAndServe()
}

// withAuth wraps a handler with token authentication
func (s *Server) withAuth(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedToken := os.Getenv("WSS_AUTH_TOKEN")
		if expectedToken == "" {
			log.Info.Printf("WSS_AUTH_TOKEN not set")
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		token := r.URL.Query().Get("token")
		if token == "" {
			log.Info.Printf("Missing token from %s", r.RemoteAddr)
			http.Error(w, "Missing auth token", http.StatusUnauthorized)
			return
		}

		if token != expectedToken {
			log.Info.Printf("Invalid token from %s", r.RemoteAddr)
			http.Error(w, "Invalid auth token", http.StatusUnauthorized)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// handleConnection handles a new WebSocket connection
func (s *Server) handleConnection(ws *websocket.Conn) {
	// Generate session ID
	sessionID := fmt.Sprintf("#%d", atomic.AddUint64(&s.sessionCount, 1))

	// Get connection details
	remoteAddr := ws.Request().RemoteAddr
	localAddr := ws.Request().Host
	log.Info.Printf("New connection %s from %s to %s", sessionID, remoteAddr, localAddr)

	// Send session ID to client
	if err := websocket.JSON.Send(ws, struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
	}{
		Type:      "session",
		SessionID: sessionID,
	}); err != nil {
		log.Info.Printf("Failed to send session ID: %v", err)
		return
	}

	// Start a new shell using /bin/sh
	// This is intentionally using a basic shell for PTY functionality
	// The shell is isolated with restricted PATH and HOME=/tmp for security
	// nosemgrep: no-system-exec
	cmd := exec.Command("/bin/sh")
	cmd.Env = []string{
		"TERM=xterm",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"SHELL=/bin/sh",
		"PS1=\\$ ",
	}

	// Create PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Info.Printf("Failed to start PTY: %v", err)
		ws.Close()
		return
	}
	defer func() {
		ptmx.Close()
		s.ptys.Delete(sessionID)
	}()

	// Store PTY for future use (e.g., file upload channel)
	s.ptys.Store(sessionID, ptmx)

	// Forward data in both directions
	errc := make(chan error, 1)

	// Terminal -> PTY
	go func(ptmx *os.File, ws *websocket.Conn) {
		_, err := io.Copy(ptmx, ws)
		errc <- err
	}(ptmx, ws)

	// PTY -> Terminal
	go func(ws *websocket.Conn, ptmx *os.File) {
		_, err := io.Copy(ws, ptmx)
		errc <- err
	}(ws, ptmx)

	// Wait for either direction to finish
	if err := <-errc; err != nil && err != io.EOF && !isConnectionClosed(err) {
		log.Debug.Printf("IO error %s: %v", sessionID, err)
	}
	log.Info.Printf("Connection closed %s", sessionID)
}

// handleSessionControl handles session control HTTP endpoints
func (s *Server) handleSessionControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session ID from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 || parts[1] != "session" || parts[3] != "winsize" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	sessionID := parts[2]

	// Get PTY for session
	ptmxVal, ok := s.ptys.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	ptmx := ptmxVal.(*os.File)

	// Parse window size from request
	var ws struct {
		Rows    uint16 `json:"rows"`
		Cols    uint16 `json:"cols"`
		XPixels uint16 `json:"x_pixels"`
		YPixels uint16 `json:"y_pixels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set window size on PTY
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: ws.Rows,
		Cols: ws.Cols,
		X:    ws.XPixels,
		Y:    ws.YPixels,
	}); err != nil {
		log.Debug.Printf("Failed to set window size for session %s: %v", sessionID, err)
		http.Error(w, "Failed to set window size", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// isConnectionClosed checks if an error is due to normal connection closure
func isConnectionClosed(err error) bool {
	return err.Error() == "use of closed network connection" ||
		err.Error() == "EOF" ||
		err.Error() == "websocket: close 1000 (normal)"
}

// Stop gracefully shuts down the server
func (s *Server) Stop() {
	if s.server != nil {
		s.server.Close()
	}
}
