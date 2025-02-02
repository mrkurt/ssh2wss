//go:build unix
// +build unix

/*
Security Testing Guidelines for H2C PTY Server

Key areas to test:
1. Authentication
   - Valid/invalid auth tokens
   - Missing tokens
   - Extremely long tokens (buffer overflow attempts)
   - Empty tokens
   - Special characters in tokens

2. HTTP/2 Connection
   - Multiple simultaneous connections
   - Rapid connect/disconnect cycles
   - Invalid protocol versions

3. PTY/Shell Security
   - Command injection attempts
   - Terminal escape sequences
   - Large data payloads
   - Special control characters
   - Shell environment manipulation

4. Resource Management
   - Memory usage under load
   - File descriptor leaks
   - Process cleanup after disconnection
   - Proper signal handling
*/

package tests

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestServerAuth(t *testing.T) {
	// Get a free port
	port, err := GetFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	// Start server in PTY
	pty := NewPTYTest(t)
	defer pty.Cleanup(t)

	// Set auth token
	authToken := "test-token"
	cmd := exec.Command(ServerBinaryPath, "server", "-port", fmt.Sprintf("%d", port))
	cmd.Env = append(os.Environ(), "WSS_AUTH_TOKEN="+authToken)

	if err := pty.StartCommand(t, cmd); err != nil {
		t.Fatal(err)
	}

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Create H2C client
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{"valid token", authToken, false},
		{"invalid token", "wrong-token", true},
		{"empty token", "", true},
		{"long token", strings.Repeat("x", 8192), true},
		{"special chars token", "!@#$%^&*()", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := fmt.Sprintf("http://localhost:%d/terminal?token=%s", port, url.QueryEscape(tt.token))
			req, err := http.NewRequest(http.MethodPost, u, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if tt.wantErr {
				if err == nil && resp.StatusCode == http.StatusOK {
					resp.Body.Close()
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected status OK, got %d", resp.StatusCode)
				} else {
					resp.Body.Close()
				}
			}
		})
	}
}
