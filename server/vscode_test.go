package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/websocket"
)

// testWSClient wraps common WebSocket test operations
type testWSClient struct {
	t      *testing.T
	conn   *websocket.Conn
	nextID int
}

func newTestClient(t *testing.T, url string) *testWSClient {
	conn, err := websocket.Dial(url, "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	return &testWSClient{t: t, conn: conn, nextID: 1}
}

func (c *testWSClient) close() {
	c.conn.Close()
}

func (c *testWSClient) sendRequest(method string, params interface{}) Message {
	id := c.nextID
	c.nextID++

	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			c.t.Fatalf("Failed to marshal params: %v", err)
		}
		paramsJSON = json.RawMessage(data)
	}

	req := Message{
		Type:   "request",
		ID:     id,
		Method: method,
		Params: paramsJSON,
	}

	if err := websocket.JSON.Send(c.conn, req); err != nil {
		c.t.Fatalf("Failed to send request: %v", err)
	}

	var response Message
	if err := websocket.JSON.Receive(c.conn, &response); err != nil {
		c.t.Fatalf("Failed to receive response: %v", err)
	}

	if response.ID != id {
		c.t.Errorf("Response ID mismatch: got %d, want %d", response.ID, id)
	}

	return response
}

func TestVSCodeServer_Handshake(t *testing.T) {
	server := httptest.NewServer(NewVSCodeServer().Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := newTestClient(t, wsURL)
	defer client.close()

	// Test handshake with all capabilities
	response := client.sendRequest("handshake", map[string]interface{}{
		"capabilities": map[string]bool{
			"remoteShell": true,
			"fileSystem":  true,
			"process":     true,
			"terminal":    true,
		},
	})

	if response.Error != nil {
		t.Errorf("Unexpected error: %v", response.Error)
	}

	var result struct {
		Capabilities struct {
			Process bool `json:"process"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if !result.Capabilities.Process {
		t.Error("Expected process capability to be true")
	}
}

func TestVSCodeServer_ProcessExec(t *testing.T) {
	server := httptest.NewServer(NewVSCodeServer().Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := newTestClient(t, wsURL)
	defer client.close()

	testCases := []struct {
		name     string
		command  string
		args     []string
		wantErr  bool
		contains string
	}{
		{
			name:     "echo command",
			command:  "echo",
			args:     []string{"hello world"},
			contains: "hello world",
		},
		{
			name:     "pwd command",
			command:  "pwd",
			contains: string(os.PathSeparator),
		},
		{
			name:     "ls command",
			command:  "ls",
			args:     []string{"-a"},
			contains: ".",
		},
		{
			name:    "invalid command",
			command: "nonexistentcommand",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response := client.sendRequest("process/exec", ProcessExecParams{
				Command: tc.command,
				Args:    tc.args,
			})

			if tc.wantErr {
				if response.Error == nil {
					t.Error("Expected error, got none")
				}
				return
			}

			if response.Error != nil {
				t.Errorf("Unexpected error: %v", response.Error)
				return
			}

			var result struct {
				ExitCode int    `json:"exitCode"`
				Stdout   string `json:"stdout"`
				Stderr   string `json:"stderr"`
			}
			if err := json.Unmarshal(response.Result, &result); err != nil {
				t.Fatalf("Failed to parse result: %v", err)
			}

			if result.ExitCode != 0 {
				t.Errorf("Expected exit code 0, got %d", result.ExitCode)
			}

			if !strings.Contains(result.Stdout, tc.contains) {
				t.Errorf("Expected stdout to contain %q, got %q", tc.contains, result.Stdout)
			}
		})
	}
}

func TestVSCodeServer_ConcurrentConnections(t *testing.T) {
	server := httptest.NewServer(NewVSCodeServer().Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	numClients := 10 // Increased from 5
	var wg sync.WaitGroup
	wg.Add(numClients)

	// Channel to collect errors from goroutines
	errCh := make(chan error, numClients)

	// Create a temporary directory for concurrent file operations
	tmpDir, err := os.MkdirTemp("", "vscode-concurrent-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for i := 0; i < numClients; i++ {
		go func(id int) {
			defer wg.Done()

			client := newTestClient(t, wsURL)
			defer client.close()

			// Test handshake
			response := client.sendRequest("handshake", map[string]interface{}{
				"capabilities": map[string]bool{"process": true},
			})
			if response.Error != nil {
				errCh <- fmt.Errorf("client %d: handshake error: %v", id, response.Error)
				return
			}

			// Test concurrent file operations
			testFile := filepath.Join(tmpDir, fmt.Sprintf("test-%d.txt", id))

			// Write to file
			response = client.sendRequest("process/exec", ProcessExecParams{
				Command: "sh",
				Args:    []string{"-c", fmt.Sprintf("echo 'content from client %d' > %s", id, testFile)},
			})
			if response.Error != nil {
				errCh <- fmt.Errorf("client %d: file write error: %v", id, response.Error)
				return
			}

			// Read the file back
			response = client.sendRequest("process/exec", ProcessExecParams{
				Command: "cat",
				Args:    []string{testFile},
			})
			if response.Error != nil {
				errCh <- fmt.Errorf("client %d: file read error: %v", id, response.Error)
				return
			}

			// List directory
			response = client.sendRequest("process/exec", ProcessExecParams{
				Command: "ls",
				Args:    []string{"-la", tmpDir},
			})
			if response.Error != nil {
				errCh <- fmt.Errorf("client %d: ls error: %v", id, response.Error)
				return
			}

			// Run a longer process
			response = client.sendRequest("process/exec", ProcessExecParams{
				Command: "sleep",
				Args:    []string{"0.1"}, // Short sleep to create overlap
			})
			if response.Error != nil {
				errCh <- fmt.Errorf("client %d: sleep error: %v", id, response.Error)
				return
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errCh)

	// Check for any errors
	for err := range errCh {
		t.Error(err)
	}
}

func TestVSCodeServer_ErrorCases(t *testing.T) {
	server := httptest.NewServer(NewVSCodeServer().Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := newTestClient(t, wsURL)
	defer client.close()

	testCases := []struct {
		name       string
		method     string
		params     interface{}
		wantErrMsg string
	}{
		{
			name:       "unknown method",
			method:     "unknown/method",
			params:     struct{}{},
			wantErrMsg: "Unknown method",
		},
		{
			name:       "invalid process params",
			method:     "process/exec",
			params:     "invalid json",
			wantErrMsg: "Invalid parameters",
		},
		{
			name:   "command with invalid path",
			method: "process/exec",
			params: ProcessExecParams{
				Command: "/nonexistent/path",
			},
			wantErrMsg: "Command failed",
		},
		{
			name:   "command with no permissions",
			method: "process/exec",
			params: ProcessExecParams{
				Command: "/etc/shadow", // Should fail on read attempt
			},
			wantErrMsg: "Command failed",
		},
		{
			name:   "empty command",
			method: "process/exec",
			params: ProcessExecParams{
				Command: "",
			},
			wantErrMsg: "Command failed",
		},
		{
			name:   "very long command",
			method: "process/exec",
			params: ProcessExecParams{
				Command: strings.Repeat("a", 10000),
			},
			wantErrMsg: "Command failed",
		},
		{
			name:   "invalid working directory",
			method: "process/exec",
			params: ProcessExecParams{
				Command: "pwd",
				Args:    []string{"/nonexistent/directory"},
			},
			wantErrMsg: "Command failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response := client.sendRequest(tc.method, tc.params)

			if response.Error == nil {
				t.Fatal("Expected error, got none")
			}

			if !strings.Contains(response.Error.Message, tc.wantErrMsg) {
				t.Errorf("Error message %q does not contain %q", response.Error.Message, tc.wantErrMsg)
			}
		})
	}

	// Test malformed JSON
	malformedJSON := `{"type":"request","id":1,method:"broken"`
	if err := websocket.Message.Send(client.conn, malformedJSON); err != nil {
		t.Fatalf("Failed to send malformed JSON: %v", err)
	}

	// Should get an error response or connection close
	var response Message
	if err := websocket.JSON.Receive(client.conn, &response); err == nil {
		if response.Error == nil {
			t.Error("Expected error for malformed JSON, got none")
		}
	}
}

// TestVSCodeServer_RapidRequests tests handling of multiple rapid requests
func TestVSCodeServer_RapidRequests(t *testing.T) {
	server := httptest.NewServer(NewVSCodeServer().Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := newTestClient(t, wsURL)
	defer client.close()

	// Send multiple requests concurrently
	numRequests := 10
	var wg sync.WaitGroup
	wg.Add(numRequests)
	errCh := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			response := client.sendRequest("process/exec", ProcessExecParams{
				Command: "echo",
				Args:    []string{fmt.Sprintf("request %d", id)},
			})
			if response.Error != nil {
				errCh <- fmt.Errorf("request %d failed: %v", id, response.Error)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestVSCodeServer_NonDestructiveCommands(t *testing.T) {
	// Create a temporary directory for file operations
	tmpDir, err := os.MkdirTemp("", "vscode-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := httptest.NewServer(NewVSCodeServer().Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := newTestClient(t, wsURL)
	defer client.close()

	// Test directory listing
	response := client.sendRequest("process/exec", ProcessExecParams{
		Command: "ls",
		Args:    []string{"-la", tmpDir},
	})
	if response.Error != nil {
		t.Errorf("ls command failed: %v", response.Error)
	}

	// Test file creation and reading
	testFile := filepath.Join(tmpDir, "test.txt")
	response = client.sendRequest("process/exec", ProcessExecParams{
		Command: "sh",
		Args:    []string{"-c", fmt.Sprintf("echo 'test content' > %s", testFile)},
	})
	if response.Error != nil {
		t.Errorf("File creation failed: %v", response.Error)
	}

	response = client.sendRequest("process/exec", ProcessExecParams{
		Command: "cat",
		Args:    []string{testFile},
	})
	if response.Error != nil {
		t.Errorf("File reading failed: %v", response.Error)
	}

	var result struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if !strings.Contains(result.Stdout, "test content") {
		t.Errorf("Expected file content 'test content', got %q", result.Stdout)
	}

	// Test environment variables
	response = client.sendRequest("process/exec", ProcessExecParams{
		Command: "env",
	})
	if response.Error != nil {
		t.Errorf("env command failed: %v", response.Error)
	}

	// Test process listing
	response = client.sendRequest("process/exec", ProcessExecParams{
		Command: "ps",
		Args:    []string{"aux"},
	})
	if response.Error != nil {
		t.Errorf("ps command failed: %v", response.Error)
	}
}
