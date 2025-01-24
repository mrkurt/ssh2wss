package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"ssh2wss/server"

	"golang.org/x/net/websocket"
)

type TerminalServer struct {
	port int
	cert string
	key  string
}

func NewTerminalServer(port int, cert, key string) *TerminalServer {
	return &TerminalServer{
		port: port,
		cert: cert,
		key:  key,
	}
}

func (s *TerminalServer) handleTerminal(ws *websocket.Conn) {
	// Create a new terminal with default size
	term, err := server.NewTerminal(80, 24)
	if err != nil {
		log.Printf("Failed to create terminal: %v", err)
		return
	}
	defer term.Close()

	// Start the terminal
	if err := term.Start(""); err != nil {
		log.Printf("Failed to start terminal: %v", err)
		return
	}

	// Handle terminal resize messages
	go func() {
		var msg struct {
			Type string `json:"type"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		for {
			if err := websocket.JSON.Receive(ws, &msg); err != nil {
				return
			}
			if msg.Type == "resize" {
				term.Resize(uint16(msg.Cols), uint16(msg.Rows))
			}
		}
	}()

	// Bidirectional copy
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := term.Read(buf)
			if err != nil {
				return
			}
			if _, err := ws.Write(buf[:n]); err != nil {
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				return
			}
			if _, err := term.Write(buf[:n]); err != nil {
				return
			}
		}
	}()

	// Wait for terminal to finish
	select {}
}

func (s *TerminalServer) Start() error {
	http.Handle("/terminal", websocket.Handler(s.handleTerminal))

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Terminal server listening on wss://*%s/terminal", addr)

	if s.cert != "" && s.key != "" {
		return http.ListenAndServeTLS(addr, s.cert, s.key, nil)
	}
	return http.ListenAndServe(addr, nil)
}

func serverMain() {
	port := flag.Int("port", 8080, "Port to listen on")
	cert := flag.String("cert", "", "TLS certificate file")
	key := flag.String("key", "", "TLS key file")
	flag.Parse()

	server := NewTerminalServer(*port, *cert, *key)
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
