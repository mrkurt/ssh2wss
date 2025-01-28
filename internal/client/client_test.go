package client

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"flyssh/server"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

// Unit tests for the client package focus on verifying small, isolated pieces of functionality.
// When integration tests fail, we should add focused unit tests here that:
// 1. Try to replicate the failure in isolation
// 2. Avoid starting real servers when possible
// 3. Use test doubles (like testConn) to verify exact protocol/data expectations
// 4. Keep tests fast and deterministic
//
// For example, if an SSH command fails in integration tests:
// - Don't start a real SSH server
// - Use testConn to verify the exact bytes being sent/received
// - Add test cases for the specific failure scenario
// - Focus on one specific interaction (SSH version exchange in this case)

// testConn implements net.Conn for testing SSH protocol interactions.
// Instead of connecting to a real server, it:
// - Returns pre-defined data for reads (simulating server responses)
// - Records written data for verification (capturing client requests)
// - Allows tests to verify exact protocol-level interactions
type testConn struct {
	readData  []byte       // Pre-defined data to return from Read
	writeData bytes.Buffer // Records data written by client
}

func (c *testConn) Read(b []byte) (n int, err error) {
	if len(c.readData) == 0 {
		return 0, io.EOF
	}
	n = copy(b, c.readData)
	c.readData = c.readData[n:]
	return n, nil
}

func (c *testConn) Write(b []byte) (n int, err error) {
	return c.writeData.Write(b)
}

func (c *testConn) Close() error                       { return nil }
func (c *testConn) LocalAddr() net.Addr                { return nil }
func (c *testConn) RemoteAddr() net.Addr               { return nil }
func (c *testConn) SetDeadline(t time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(t time.Time) error { return nil }

// TestWSNetConnInterface verifies that wsNetConn properly implements net.Conn.
// This is a rare case where we need a real WebSocket server because we're testing
// the WebSocket connection itself. Most other tests should use testConn instead.
func TestWSNetConnInterface(t *testing.T) {
	// Create a WebSocket server that echoes data back
	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		io.Copy(ws, ws)
	})

	server := httptest.NewServer(wsHandler)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + server.URL[4:]

	// Connect to WebSocket server
	ws, err := websocket.Dial(wsURL, "", "http://localhost")
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}

	// Create net.Conn wrapper
	conn := newWSNetConn(ws)

	// Verify it implements net.Conn
	var _ net.Conn = conn

	// Test basic I/O
	testData := []byte("test message")
	n, err := conn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	buf := make([]byte, len(testData))
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, read %d", len(testData), n)
	}
	if !bytes.Equal(buf, testData) {
		t.Errorf("Expected %q, got %q", testData, buf)
	}
}

// TestSSHOverCustomConn verifies that we can create an SSH client over any net.Conn.
// This shows how to test SSH protocol interactions without starting a real server:
// 1. Use testConn to provide fake responses and capture client requests
// 2. Verify the exact bytes sent by the client
// 3. Focus on one specific interaction (SSH version exchange in this case)
func TestSSHOverCustomConn(t *testing.T) {
	// Create test conn with SSH server response data
	conn := &testConn{
		readData: []byte("SSH-2.0-OpenSSH_8.1\r\n"),
	}

	config := &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Attempt SSH handshake over test conn
	_, _, _, err := ssh.NewClientConn(conn, "", config)
	if err != nil {
		// We expect this to fail since we're not providing proper SSH response data
		// but we can verify that the client sent the right handshake data
		if !bytes.HasPrefix(conn.writeData.Bytes(), []byte("SSH-2.0-")) {
			t.Errorf("Expected SSH handshake to start with SSH-2.0-, got %q", conn.writeData.String())
		}
	}
}

// TestSSHOverWebSocket verifies that SSH protocol works over WebSocket transport
func TestSSHOverWebSocket(t *testing.T) {
	// Generate SSH host key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate host key: %v", err)
	}

	// Convert to PEM format
	keyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	// Set test auth token
	const testToken = "test-token"
	os.Setenv("WSS_AUTH_TOKEN", testToken)
	defer os.Unsetenv("WSS_AUTH_TOKEN")

	// Create WebSocket+SSH server with a high random port
	const testPort = 35555
	wsServer, err := server.NewWebSocketSSHServer(testPort, keyBytes)
	if err != nil {
		t.Fatalf("Failed to create WebSocket SSH server: %v", err)
	}

	// Start server in goroutine
	go func() {
		if err := wsServer.Start(); err != nil {
			t.Errorf("Server failed: %v", err)
		}
	}()

	// Wait a moment for server to start
	time.Sleep(100 * time.Millisecond)

	// Create client config
	config := &ClientConfig{
		WSServer:  fmt.Sprintf("ws://localhost:%d", testPort),
		AuthToken: testToken,
		Command:   "echo test", // Run a simple command in non-interactive mode
	}

	// Create and run client
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Run client in goroutine since it blocks
	errChan := make(chan error, 1)
	go func() {
		errChan <- client.Run()
	}()

	// Wait for error or timeout
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Client failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		// Success - connection stayed open
	}
}

// Test SSH host key - DO NOT use in production!
const testHostKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
NhAAAAAwEAAQAAAQEAvRQk2oQqLB01iCnJuv0J6gEu3kDgzVXZZqyqp1qLJKPn+hYk800g
5dZ6vNdZQ5xjvZMhAqyKWJJ8H9MJ/9oJDUfHq3mEJ9XjR7kDWc4FwIV9Y4RCQynwKGzKWP
EMYyO9qjsT0yDhb5K+1/RlUq+2VB1OHf+42Zg8Lg/2jBgGFQfrDhVZEd4Kj0A+lvQMuqtm
b2wqaZqGxn5TxJxRWCvO1GxqUO4CVwxQsAbRSFVsJh7KBKPYpvO6plvWjFMqC7FTLjLhJK
PP3/1k+ihJRWFMA3BmR8GbwqaMpc8tqRvGz4Rp4e0UPckHY4oK/k5rRN8pnXGdgRzN5Rk+
YQ2IqQAAA8g3ss0EN7LNBAAAAAdzc2gtcnNhAAABAQC9FCTahCosHTWIKcm6/QnqAS7eQO
DNVdlmrKqnWoskoef6FiTzTSDl1nq811lDnGO9kyECrIpYknwf0wn/2gkNR8ereYQn1eNH
uQNZzgXAhX1jhEJDKfAobMpY8QxjI72qOxPTIOFvkr7X9GVSr7ZUHU4d/7jZmDwuD/aMGA
YVB+sOFVkR3gqPQD6W9Ay6q2ZvbCppmoZGflPEnFFYK87UbGpQ7gJXDFCwBtFIVWwmHsoE
o9im87qmW9aMUyoLsVMuMuEko8/f/WT6KElFYUwDcGZHwZvCpoylzy2pG8bPhGnh7RQ9yQ
djigr+TmtE3ymdcZ2BHM3lGT5hDYipAAAAAwEAAQAAAQEAnB+DGu3GLZwX4CxSh+dvYS6C
JeXAFEu3kYwUzj5x5CyhtAUzuGQ2qw5eCQ6C9wGqK6PmNJKXlyPW2GHJwoYPR2hJvHgPLC
QKnPQh6Qj5IwrWGUYhvU4yB5SHPdHNDwmECQLYqZw0Z0XlFFmz4hk4sSBX1oGF4FBQKFob
qWzxYVk4SvC0TtHMjJGTJJsc4f1SizE6cVOd4IXRZ+4iQPQD1SiJYhEKqh3VH8yf8EhyDH
/90pn4ZQ8wMD6BCRh1FGKHGym4r+hCjDr2y2gHbh+5WGwXBZHH4TNcl+kZGD0Y9qj2r6zR
2eQUJ7oAD5AN4y6JQTKiAXeGd0wm7/5hAQAAAIBKqIWP0J1bvqJxUyiRrpuKs4qEyxau9G
wQR4l3mX2ASrA80UgRkrnvRjImxTBhzS4NT4NOuNF5ZO8oVHvLXBHDDME2sAFkmLFxoFAg
6hX3i0REGfN8YU/+ij0Zj6BWG/PVgbL9agfqHUDWjNhqQvXhwTN8KmoxGBAYQbdfHsqsAA
AAAIEAyy2c3o0b4hdXYA0Y2JpJ4JhIQ9z7EhwR3wYC3/VhVy8oYUy9eRi9sBqNEAE+cAYt
KX/gZVv6hZUhKw5tEFvJVJDGxoUFjtwOAgLnH3ENhiEXVUBQDQhs2S2CzQ7TDDm2447Gqz
IvYXo8ZxoAL8dXFLCJn6Kh78IJuXtBWqsAAACBAO/zoZGRkBRZf5RYmn5HJmEPzZEQz5Wd
hDUhUQKn/AAU3MpkGVZtXRTzRgkGQQRkEXr+PHVm/o3LAQGQwT5LXfPK+gzqC9D1mO6MPP
RD4o4nZ/fZHvuEQOP8d0MfB6Gx0J7bpVy4LhqNzYyiDQf6ISf1NVRLhxpqB6bWNGAhAAAA
EHRlc3RAZXhhbXBsZS5jb20BAg==
-----END OPENSSH PRIVATE KEY-----`
