package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sync"

	"golang.org/x/net/websocket"
)

// Message represents a VSCode protocol message
type Message struct {
	Type   string          `json:"type"`             // request, response, or event
	ID     int             `json:"id,omitempty"`     // message ID for requests/responses
	Method string          `json:"method"`           // operation to perform
	Params json.RawMessage `json:"params,omitempty"` // operation parameters
	Result json.RawMessage `json:"result,omitempty"` // response result
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

// VSCodeServer handles VSCode remote development protocol
type VSCodeServer struct {
	sessions sync.Map // map[string]*websocket.Conn
}

// NewVSCodeServer creates a new VSCode protocol server
func NewVSCodeServer() *VSCodeServer {
	return &VSCodeServer{}
}

// Handler returns a websocket handler for the VSCode protocol
func (s *VSCodeServer) Handler() http.Handler {
	return websocket.Handler(s.handleConnection)
}

func (s *VSCodeServer) handleConnection(conn *websocket.Conn) {
	defer conn.Close()
	log.Printf("New VSCode connection from %s", conn.Request().RemoteAddr)

	// Handle incoming messages
	for {
		var msg Message
		if err := websocket.JSON.Receive(conn, &msg); err != nil {
			log.Printf("Failed to read message: %v", err)
			return
		}

		s.handleMessage(conn, &msg)
	}
}

func (s *VSCodeServer) handleMessage(conn *websocket.Conn, msg *Message) {
	switch msg.Method {
	case "handshake":
		s.handleHandshake(conn, msg)
	case "process/exec":
		s.handleProcessExec(conn, msg)
	default:
		s.sendError(conn, msg.ID, 404, fmt.Sprintf("Unknown method: %s", msg.Method))
	}
}

func (s *VSCodeServer) handleHandshake(conn *websocket.Conn, msg *Message) {
	// Send capabilities
	response := Message{
		Type: "response",
		ID:   msg.ID,
		Result: json.RawMessage(`{
			"capabilities": {
				"process": true
			}
		}`),
	}

	if err := websocket.JSON.Send(conn, response); err != nil {
		log.Printf("Failed to send handshake response: %v", err)
	}
}

type ProcessExecParams struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func (s *VSCodeServer) handleProcessExec(conn *websocket.Conn, msg *Message) {
	var params ProcessExecParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(conn, msg.ID, 400, "Invalid parameters")
		return
	}

	cmd := exec.Command(params.Command, params.Args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.sendError(conn, msg.ID, 500, fmt.Sprintf("Command failed: %v", err))
		return
	}

	response := Message{
		Type: "response",
		ID:   msg.ID,
		Result: json.RawMessage(fmt.Sprintf(`{
			"exitCode": 0,
			"stdout": %q,
			"stderr": ""
		}`, string(output))),
	}

	if err := websocket.JSON.Send(conn, response); err != nil {
		log.Printf("Failed to send exec response: %v", err)
	}
}

func (s *VSCodeServer) sendError(conn *websocket.Conn, id int, code int, message string) {
	response := Message{
		Type: "response",
		ID:   id,
		Error: &struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data,omitempty"`
		}{
			Code:    code,
			Message: message,
		},
	}

	if err := websocket.JSON.Send(conn, response); err != nil {
		log.Printf("Failed to send error response: %v", err)
	}
}
