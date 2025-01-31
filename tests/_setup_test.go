//go:build unix
// +build unix

package tests

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	// Cleanup
	os.RemoveAll("tmp")
	os.Exit(code)
}
