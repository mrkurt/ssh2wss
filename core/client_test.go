package core

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	url := "ws://localhost:8080"
	token := "test-token"
	client := NewClient(url, token)

	if client.url != url {
		t.Errorf("Expected URL %s, got %s", url, client.url)
	}
	if client.authToken != token {
		t.Errorf("Expected token %s, got %s", token, client.authToken)
	}
}

func TestSetIO(t *testing.T) {
	client := NewClient("ws://localhost:8080", "test-token")
	stdin := strings.NewReader("test input")
	stdout := &bytes.Buffer{}

	client.SetIO(stdin, stdout)

	if client.stdin != stdin {
		t.Error("stdin not set correctly")
	}
	if client.stdout != stdout {
		t.Error("stdout not set correctly")
	}
}
