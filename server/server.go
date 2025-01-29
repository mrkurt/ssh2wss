package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"golang.org/x/net/websocket"
)

// Message represents a WebSocket message
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// WindowSize represents terminal window dimensions
type WindowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// Server represents a WebSocket terminal server
type Server struct {
	port     int
	mux      *http.ServeMux
	server   *http.Server
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// New creates a new Server instance
func New(port int) *Server {
	mux := http.NewServeMux()
	return &Server{
		port:     port,
		mux:      mux,
		shutdown: make(chan struct{}),
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}
}

// Start begins listening for WebSocket connections
func (s *Server) Start() error {
	s.mux.Handle("/", websocket.Handler(s.handleConnection))
	log.Printf("Starting WebSocket server on %s", s.server.Addr)

	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	close(s.shutdown)
	err := s.server.Shutdown(context.Background())
	s.wg.Wait() // Wait for all connections to finish
	return err
}

// handleConnection manages a WebSocket connection
func (s *Server) handleConnection(ws *websocket.Conn) {
	s.wg.Add(1)
	defer s.wg.Done()

	log.Printf("New connection from %s", ws.RemoteAddr())

	// Verify auth token
	token := ws.Request().URL.Query().Get("token")
	if token != os.Getenv("WSS_AUTH_TOKEN") {
		log.Printf("Invalid token from %s", ws.RemoteAddr())
		ws.Close()
		return
	}

	// Create command
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm")

	// Start command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start command: %v", err)
		return
	}
	defer ptmx.Close()

	// Create done channel for cleanup
	done := make(chan struct{})
	defer close(done)

	// Handle input in a separate goroutine
	go func() {
		defer ptmx.Close()
		for {
			select {
			case <-s.shutdown:
				return
			case <-done:
				return
			default:
				// Try to read as JSON first
				var msg Message
				err := websocket.JSON.Receive(ws, &msg)
				if err == nil {
					log.Printf("Received JSON message type=%s from %s", msg.Type, ws.RemoteAddr())
					// Handle JSON message
					switch msg.Type {
					case "resize":
						var size WindowSize
						if err := json.Unmarshal(msg.Data, &size); err != nil {
							log.Printf("Failed to parse window size: %v", err)
							continue
						}

						log.Printf("Setting window size to %dx%d for %s", size.Cols, size.Rows, ws.RemoteAddr())
						if err := pty.Setsize(ptmx, &pty.Winsize{
							Rows: size.Rows,
							Cols: size.Cols,
						}); err != nil {
							log.Printf("Failed to set window size: %v", err)
						}
					}
					continue
				}

				// If not JSON, try to read as raw data
				var data []byte
				err = websocket.Message.Receive(ws, &data)
				if err != nil {
					if err != io.EOF {
						log.Printf("Failed to receive data from %s: %v", ws.RemoteAddr(), err)
					}
					return
				}

				log.Printf("Server received raw data from client: %q", data)
				// Write raw data to PTY
				if _, err := ptmx.Write(data); err != nil {
					log.Printf("Failed to write to PTY: %v", err)
					return
				}
			}
		}
	}()

	// Copy output from PTY to WebSocket
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-s.shutdown:
			return
		case <-done:
			return
		default:
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Failed to read from PTY: %v", err)
				}
				return
			}
			log.Printf("Server sending to client: %q", buf[:n])
			if _, err := ws.Write(buf[:n]); err != nil {
				log.Printf("Failed to write to WebSocket: %v", err)
				return
			}
		}
	}
}
