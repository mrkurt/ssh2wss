package terminal

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"
)

func TestCharacterSets(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		expected string
	}{
		{
			name: "ASCII Set",
			commands: []string{
				// Select ASCII character set (default)
				fmt.Sprintf("%s(B", ESC),
				"echo 'ASCII text'",
			},
			expected: "ASCII text",
		},
		{
			name: "Special Characters",
			commands: []string{
				// Test various special characters
				"echo '!@#$%^&*()'",
			},
			expected: "!@#$%^&*()",
		},
		{
			name: "Control Characters",
			commands: []string{
				// Test some control characters (bell, tab, etc)
				"echo -e '\\a\\t\\r\\n'",
			},
			expected: "\t\r\n",
		},
		{
			name: "Mixed Content",
			commands: []string{
				// Mix of ASCII, special chars, and control chars
				"echo -e 'Normal\\tTabbed\\nNewline'",
			},
			expected: "Normal\tTabbed\nNewline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Logf("Server logs: %s", getServerLogs())
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.expected)) {
				t.Errorf("Expected output to contain %q, got %q", tt.expected, string(cleanOutput))
			}
		})
	}
}

func TestControlCharacterHandling(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		expected string
	}{
		{
			name: "Backspace",
			commands: []string{
				"stty raw", // Enable raw mode for control chars
				"echo -n 'abc'",
				"echo -e '\\b\\b'", // Two backspaces
				"echo -n 'XY'",     // Replace last two chars
			},
			expected: "aXY",
		},
		{
			name: "Carriage Return",
			commands: []string{
				"stty raw", // Enable raw mode for control chars
				"echo -n 'old text'",
				"echo -e '\\r'", // Return to start of line
				"echo -n 'new'", // Overwrite beginning
			},
			expected: "new text",
		},
		{
			name: "Form Feed",
			commands: []string{
				"echo -e '\\f'", // Form feed
			},
			expected: "\f",
		},
		{
			name: "Vertical Tab",
			commands: []string{
				"echo -e 'line1\\vline2'",
			},
			expected: "line1\vline2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(t, tt.commands)
			if err != nil {
				t.Logf("Server logs: %s", getServerLogs())
				t.Fatalf("Command failed: %v\nOutput: %s", err, output)
			}

			cleanOutput := stripANSI(output)
			if !bytes.Contains(cleanOutput, []byte(tt.expected)) {
				t.Errorf("Expected output to contain %q, got %q", tt.expected, string(cleanOutput))
			}
		})
	}
}

func TestExtendedCharacters(t *testing.T) {
	binPath := "../../cmd/flyssh/flyssh"

	tests := []struct {
		name     string
		commands []string
		expected string
	}{
		{
			name: "Box Drawing",
			commands: []string{
				// Use box drawing characters
				"echo -e '┌─┐\\n│ │\\n└─┘'",
			},
			expected: "┌─┐\n│ │\n└─┘",
		},
		{
			name: "Mathematical Symbols",
			commands: []string{
				// Use mathematical symbols
				"echo '∑∏∆√∞≠≤≥'",
			},
			expected: "∑∏∆√∞≠≤≥",
		},
		{
			name: "Currency Symbols",
			commands: []string{
				// Use currency symbols
				"echo '€£¥¢₹'",
			},
			expected: "€£¥¢₹",
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
