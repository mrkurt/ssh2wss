package terminal

import (
	"bytes"
	"testing"
)

func TestCursorOperations(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Absolute Positioning",
			commands: []string{
				"echo -n 'Cursor'",
				"printf '\\033[5G'",
				"echo -n 'or'",
			},
			want: "Cursor",
		},
		{
			name: "Relative Movement",
			commands: []string{
				"echo -n 'X'",
				"printf '\\033[2C'",
				"echo -n 'Y'",
				"printf '\\033[1B'",
				"echo -n 'Z'",
			},
			want: "X  Y\nZ",
		},
		{
			name: "Save and Restore Position",
			commands: []string{
				"echo -n 'A'",
				"printf '\\033[s'",
				"printf '\\033[5C'",
				"echo -n 'B'",
				"printf '\\033[40C'",
				"echo -n 'C'",
				"printf '\\033[u'",
			},
			want: "A    B                                        C",
		},
		{
			name: "Tab Stops",
			commands: []string{
				"echo -n 'A\tB\tC'",
			},
			want: "A       B       C",
		},
		{
			name: "Complex Movement",
			commands: []string{
				"echo '1234'",
				"printf '\\033[1G'",
				"printf '\\033[1B'",
				"echo -n 'X'",
			},
			want: "1234\nX",
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

func TestCursorWrapping(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Auto Wrap at End of Line",
			commands: []string{
				"printf '\\033[79G'",
				"echo -n 'AB'", // Should wrap after A
			},
			want: "A\nB",
		},
		{
			name: "Wrap Mode Disabled",
			commands: []string{
				"printf '\\033[?7l'", // Disable wrap
				"printf '\\033[79G'", // Move to end of line
				"echo -n 'AB'",       // A should overwrite B
				"printf '\\033[?7h'", // Re-enable wrap
			},
			want: "B",
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
