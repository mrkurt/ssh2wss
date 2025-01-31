package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
	// Set up WebSocket handlers with auth wrapper
	dataHandler := websocket.Handler(s.handleConnection)
	controlHandler := websocket.Handler(s.handleControl)
	s.mux.Handle("/", s.withAuth(dataHandler))
	s.mux.Handle("/control", s.withAuth(controlHandler))

	// Start HTTP server
	addr := fmt.Sprintf(":%d", s.port)
	log.Info.Printf("Starting WebSocket server on %s", addr)
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

	// Start a new shell like an SSH server would:
	// 1. Using /bin/sh as the system shell
	// 2. Setting a minimal, controlled environment
	// 3. Matching standard SSH server behavior
	cmd := exec.Command("/bin/sh") // nosemgrep: no-system-exec
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

	// Store PTY for resize handling
	s.ptys.Store(sessionID, ptmx)

	// Set initial size to something reasonable
	pty.Setsize(ptmx, &pty.Winsize{
		Rows: 24,
		Cols: 80,
	})

	// Copy WebSocket -> PTY
	go func(ptmx *os.File, ws *websocket.Conn, cmd *exec.Cmd) {
		buf := make([]byte, 32*1024)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				if err != io.EOF && !isConnectionClosed(err) {
					log.Debug.Printf("WebSocket read error %s on %s->%s: %v", sessionID, remoteAddr, localAddr, err)
				}
				cmd.Process.Kill()
				return
			}
			if n > 0 {
				if _, err := ptmx.Write(buf[:n]); err != nil {
					if err != io.EOF {
						log.Debug.Printf("PTY write error %s on %s->%s: %v", sessionID, remoteAddr, localAddr, err)
					}
					cmd.Process.Kill()
					return
				}
			}
		}
	}(ptmx, ws, cmd)

	// Copy PTY -> WebSocket
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Debug.Printf("PTY read error %s on %s->%s: %v", sessionID, remoteAddr, localAddr, err)
			}
			return
		}
		if n > 0 {
			if _, err := ws.Write(buf[:n]); err != nil {
				if err != io.EOF && !isConnectionClosed(err) {
					log.Debug.Printf("WebSocket write error %s on %s->%s: %v", sessionID, remoteAddr, localAddr, err)
				}
				return
			}
		}
	}
}

// isConnectionClosed checks if an error is due to normal connection closure
func isConnectionClosed(err error) bool {
	return err.Error() == "use of closed network connection" ||
		err.Error() == "EOF" ||
		err.Error() == "websocket: close 1000 (normal)"
}

// handleControl handles control messages like window resizing
func (s *Server) handleControl(ws *websocket.Conn) {
	remoteAddr := ws.Request().RemoteAddr
	localAddr := ws.Request().Host

	// Wait for first message to get session ID
	var msg struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Data      struct {
			Rows uint16 `json:"rows"`
			Cols uint16 `json:"cols"`
		} `json:"data"`
	}

	if err := websocket.JSON.Receive(ws, &msg); err != nil {
		if err != io.EOF {
			log.Debug.Printf("Control message error: %v", err)
		}
		return
	}

	log.Info.Printf("New control connection %s from %s to %s\n", msg.SessionID, remoteAddr, localAddr)

	// Handle first message
	if msg.Type == "resize" {
		if ptmx, ok := s.ptys.Load(msg.SessionID); ok {
			if err := pty.Setsize(ptmx.(*os.File), &pty.Winsize{
				Rows: msg.Data.Rows,
				Cols: msg.Data.Cols,
			}); err != nil {
				log.Debug.Printf("Failed to resize PTY %s: %v", msg.SessionID, err)
			}
		}
	}

	// Handle subsequent messages
	for {
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				log.Debug.Printf("Control message error %s: %v", msg.SessionID, err)
			}
			return
		}

		if msg.Type == "resize" {
			if ptmx, ok := s.ptys.Load(msg.SessionID); ok {
				if err := pty.Setsize(ptmx.(*os.File), &pty.Winsize{
					Rows: msg.Data.Rows,
					Cols: msg.Data.Cols,
				}); err != nil {
					log.Debug.Printf("Failed to resize PTY %s: %v", msg.SessionID, err)
				}
			}
		}
	}
}
