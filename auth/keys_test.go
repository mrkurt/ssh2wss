package auth

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateKey(t *testing.T) {
	// Test that the same seed generates the same key
	seed1 := "test-seed-1"
	key1, err := GenerateKey(seed1)
	if err != nil {
		t.Fatalf("Failed to generate first key: %v", err)
	}

	key2, err := GenerateKey(seed1)
	if err != nil {
		t.Fatalf("Failed to generate second key: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("Keys generated for the same seed are different")
	}

	// Test that different seeds generate different keys
	seed2 := "test-seed-2"
	key3, err := GenerateKey(seed2)
	if err != nil {
		t.Fatalf("Failed to generate key for different seed: %v", err)
	}

	if bytes.Equal(key1, key3) {
		t.Error("Keys generated for different seeds are the same")
	}

	// Test that the generated key is valid SSH private key
	_, err = ssh.ParsePrivateKey(key1)
	if err != nil {
		t.Errorf("Generated key is not a valid SSH private key: %v", err)
	}
}
