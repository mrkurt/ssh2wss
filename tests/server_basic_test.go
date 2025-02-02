//go:build unix
// +build unix

package tests

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestServerConnection(t *testing.T) {
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

	// Test valid connection
	url := fmt.Sprintf("http://localhost:%d/terminal?token=%s", port, authToken)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK, got %d", resp.StatusCode)
	}
}
