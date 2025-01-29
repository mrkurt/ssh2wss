package terminal

import (
	"bytes"
	"testing"
)

func TestWindowOperations(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Set Window Title",
			commands: []string{
				"printf '\\033]0;Test Window\\007'",
				"echo 'content'",
			},
			want: "content",
		},
		{
			name: "Window Size Report",
			commands: []string{
				"printf '\\033[18t'",
				"echo 'after size request'",
			},
			want: "after size request",
		},
		{
			name: "Multiple Window Operations",
			commands: []string{
				"printf '\\033]0;Test Window\\007'",
				"printf '\\033[18t'",
				"echo 'operations complete'",
			},
			want: "operations complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			filteredOutput := filterServerLogs(output)
			cleanOutput := stripANSI(filteredOutput)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}
		})
	}
}

func TestWindowResize(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Resize Event Handling",
			commands: []string{
				"printf '\\033[8;24;80t'",
				"echo 'after resize'",
			},
			want: "after resize",
		},
		{
			name: "Multiple Resize Events",
			commands: []string{
				"printf '\\033[8;24;80t'",
				"echo 'first'",
				"printf '\\033[8;30;100t'",
				"echo 'second'",
			},
			want: "first\nsecond",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			filteredOutput := filterServerLogs(output)
			cleanOutput := stripANSI(filteredOutput)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}
		})
	}
}

func TestWindowSizeResponse(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Request Window Size",
			commands: []string{
				"printf '\\033[14t'",
				"echo 'size requested'",
			},
			want: "size requested",
		},
		{
			name: "Request Text Area Size",
			commands: []string{
				"printf '\\033[19t'",
				"echo 'area requested'",
			},
			want: "area requested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			filteredOutput := filterServerLogs(output)
			cleanOutput := stripANSI(filteredOutput)
			if !bytes.Contains(cleanOutput, []byte(tt.want)) {
				t.Errorf("Expected output to contain %q, got %q", tt.want, string(cleanOutput))
			}

			// Check for size response in raw output
			if tt.name == "Request Window Size" {
				if !bytes.Contains(output, []byte("\\033[4;")) {
					t.Error("Expected window size response not found in output")
				}
			}
		})
	}
}
