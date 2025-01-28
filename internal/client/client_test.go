package client

import (
	"bytes"
	"io"
	"net"
	"net/http/httptest"
	"testing"
	"time"

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
