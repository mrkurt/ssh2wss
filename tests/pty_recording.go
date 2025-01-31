//go:build !windows
// +build !windows

package tests

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// NOTE: The following types are duplicated in windows_terminal_test.go
// This duplication is necessary because windows_terminal_test.go is conditionally built with //go:build windows
// while this file is used for non-Windows builds. Both files need these types
// but can't share them due to build constraints.

// PTYRecording represents a recorded PTY session
type PTYRecording struct {
	Interactions []PTYInteraction `json:"interactions"`
}

// PTYInteraction represents a single input/output pair
type PTYInteraction struct {
	Input      []byte     `json:"input"`      // What was sent to PTY
	Output     []byte     `json:"output"`     // What PTY responded with
	Delay      float64    `json:"delay"`      // Delay before this interaction in seconds
	WindowSize WindowSize `json:"windowSize"` // Terminal dimensions at time of interaction
}

// WindowSize represents terminal dimensions
type WindowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// loadPTYRecording loads and decodes the test recording
func loadPTYRecording(t testing.TB) (*PTYRecording, error) {
	data, err := os.ReadFile("tests/fixtures/pty_recording.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read recording: %v", err)
	}

	var recording struct {
		Interactions []struct {
			Input      string  `json:"input"`
			Output     string  `json:"output"`
			Delay      float64 `json:"delay"`
			WindowSize struct {
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			} `json:"windowSize"`
		} `json:"interactions"`
	}

	if err := json.Unmarshal(data, &recording); err != nil {
		return nil, fmt.Errorf("failed to parse recording: %v", err)
	}

	// Convert base64 strings to bytes
	result := &PTYRecording{
		Interactions: make([]PTYInteraction, len(recording.Interactions)),
	}

	for i, interaction := range recording.Interactions {
		input, err := base64.StdEncoding.DecodeString(interaction.Input)
		if err != nil {
			return nil, fmt.Errorf("failed to decode input %d: %v", i, err)
		}
		output, err := base64.StdEncoding.DecodeString(interaction.Output)
		if err != nil {
			return nil, fmt.Errorf("failed to decode output %d: %v", i, err)
		}
		result.Interactions[i].Input = input
		result.Interactions[i].Output = output
		result.Interactions[i].Delay = interaction.Delay
		result.Interactions[i].WindowSize.Rows = interaction.WindowSize.Rows
		result.Interactions[i].WindowSize.Cols = interaction.WindowSize.Cols
	}

	return result, nil
}
