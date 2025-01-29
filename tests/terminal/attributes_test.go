package terminal

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"
)

func TestCharacterAttributes(t *testing.T) {
	binPath := "../../cmd/flyssh/flyssh"

	tests := []struct {
		name     string
		sequence string
		text     string
		expected string
	}{
		{
			name:     "Bold Text",
			sequence: fmt.Sprintf("%s1m", CSI),
			text:     "Bold Text",
			expected: "Bold Text",
		},
		{
			name:     "Dim Text",
			sequence: fmt.Sprintf("%s2m", CSI),
			text:     "Dim Text",
			expected: "Dim Text",
		},
		{
			name:     "Underlined Text",
			sequence: fmt.Sprintf("%s4m", CSI),
			text:     "Underlined Text",
			expected: "Underlined Text",
		},
		{
			name:     "Blinking Text",
			sequence: fmt.Sprintf("%s5m", CSI),
			text:     "Blinking Text",
			expected: "Blinking Text",
		},
		{
			name:     "Reverse Video",
			sequence: fmt.Sprintf("%s7m", CSI),
			text:     "Reverse Video",
			expected: "Reverse Video",
		},
		{
			name:     "Multiple Attributes",
			sequence: fmt.Sprintf("%s1;4;5m", CSI), // Bold + Underline + Blink
			text:     "Multiple Styles",
			expected: "Multiple Styles",
		},
		{
			name:     "Reset Attributes",
			sequence: fmt.Sprintf("%s1m%s%s0m%s", CSI, "Bold", CSI, "Normal"),
			text:     "",
			expected: "BoldNormal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := fmt.Sprintf("echo -e '%s%s%s'", tt.sequence, tt.text, Reset)
			cmd := exec.Command(binPath, "client",
				"-dev",
				"-f", script)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			// Strip ANSI sequences for comparison
			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.expected)) {
				t.Errorf("Expected output to contain %q, got %q", tt.expected, string(cleanOutput))
			}

			// Verify ANSI sequence is present in raw output
			if !bytes.Contains(output, []byte(tt.sequence)) {
				t.Errorf("Expected raw output to contain ANSI sequence %q", tt.sequence)
			}
		})
	}
}

func TestCombinedAttributes(t *testing.T) {
	binPath := "../../cmd/flyssh/flyssh"

	tests := []struct {
		name     string
		commands []string
		expected string
	}{
		{
			name: "Bold + Color",
			commands: []string{
				fmt.Sprintf("echo -e '%s1;31mBold Red%s'", CSI, Reset),
			},
			expected: "Bold Red",
		},
		{
			name: "Underline + Color",
			commands: []string{
				fmt.Sprintf("echo -e '%s4;32mUnderlined Green%s'", CSI, Reset),
			},
			expected: "Underlined Green",
		},
		{
			name: "Bold + Underline + Color",
			commands: []string{
				fmt.Sprintf("echo -e '%s1;4;34mBold Underlined Blue%s'", CSI, Reset),
			},
			expected: "Bold Underlined Blue",
		},
		{
			name: "Multiple Styles With Reset",
			commands: []string{
				fmt.Sprintf("echo -e '%s1mBold%s0m Normal%s1;31m Bold Red%s'", CSI, CSI, CSI, Reset),
			},
			expected: "Bold Normal Bold Red",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := joinCommands(tt.commands)
			cmd := exec.Command(binPath, "client",
				"-dev",
				"-f", script)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.expected)) {
				t.Errorf("Expected output to contain %q, got %q", tt.expected, string(cleanOutput))
			}
		})
	}
}
