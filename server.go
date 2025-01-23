package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/net/websocket"
	"golang.org/x/term"
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

type winsize struct {
	rows    uint16
	cols    uint16
	xpixels uint16
	ypixels uint16
}

func setWinsize(f *os.File, w, h int) {
	ws := &winsize{
		rows: uint16(h),
		cols: uint16(w),
	}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}

func (s *TerminalServer) handleTerminal(ws *websocket.Conn) {
	// Start a new terminal session
	cmd := exec.Command("/bin/bash")

	// Create a PTY
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		log.Printf("Failed to open PTY: %v", err)
		return
	}
	defer ptmx.Close()

	// Put terminal into raw mode
	oldState, err := term.MakeRaw(int(ptmx.Fd()))
	if err != nil {
		log.Printf("Failed to set raw mode: %v", err)
		return
	}
	defer term.Restore(int(ptmx.Fd()), oldState)

	// Set initial terminal size
	setWinsize(ptmx, 80, 24)

	// Start the command with the PTY
	cmd.Stdin = ptmx
	cmd.Stdout = ptmx
	cmd.Stderr = ptmx
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    int(ptmx.Fd()),
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start command: %v", err)
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
				setWinsize(ptmx, msg.Cols, msg.Rows)
			}
		}
	}()

	// Bidirectional copy
	go func() {
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
	}()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				return
			}
			if _, err := ptmx.Write(buf[:n]); err != nil {
				return
			}
		}
	}()

	// Wait for command to finish
	cmd.Wait()
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
