package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBridge(t *testing.T) {
	defer trackOperation("TestBridge")()
	defer checkGoroutineLeak(t)()

	// Generate test host key
	hostKey, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate host key: %v", err)
	}

	// Create bridge
	bridge, err := NewBridge(2222, 8080, hostKey)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// Create a channel to signal bridge startup
	bridgeReady := make(chan struct{})

	// Start bridge in goroutine
	go func() {
		defer trackOperation("Bridge.Start")()

		// Signal when the bridge is ready
		go func() {
			// Wait for both servers to be ready
			for {
				if bridge.sshServer != nil && bridge.wsServer != nil {
					close(bridgeReady)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()

		if err := bridge.Start(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Bridge start failed: %v", err)
		}
	}()

	// Wait for bridge to be ready
	select {
	case <-bridgeReady:
		t.Log("Bridge is ready")
	case <-time.After(5 * time.Second):
		t.Fatal("Bridge failed to start within timeout")
	}

	// Run subtests
	t.Run("Command_Execution", func(t *testing.T) {
		defer trackOperation("Command_Execution")()

		// Track SSH connection
		func() {
			defer trackOperation("SSH Connection")()
			// ... existing SSH connection code ...
		}()

		// Track command execution
		func() {
			defer trackOperation("Command Run")()
			// ... existing command execution code ...
		}()
	})

	// Stop bridge
	defer func() {
		defer trackOperation("Bridge.Stop")()
		bridge.Stop()
	}()
}

func TestInteractiveSSH(t *testing.T) {
	defer trackOperation("TestInteractiveSSH")()
	// ... rest of test ...
}

func TestInteractiveSSHWithSubprocess(t *testing.T) {
	defer trackOperation("TestInteractiveSSHWithSubprocess")()
	// ... rest of test ...
}

func TestWindowResize(t *testing.T) {
	defer trackOperation("TestWindowResize")()
	// ... rest of test ...
}
