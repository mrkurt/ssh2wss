package terminal

import (
	"bytes"
	"testing"
)

func TestLineOperations(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Insert Line",
			commands: []string{
				"echo '1\n2'",
				"printf '\\033[1G'",
				"printf '\\033[L'",
				"echo -n 'new'",
			},
			want: "1\nnew\n2",
		},
		{
			name: "Delete Line",
			commands: []string{
				"echo '1\n2\n3'",
				"printf '\\033[2G'",
				"printf '\\033[M'",
			},
			want: "1\n3",
		},
		{
			name: "Multiple Line Operations",
			commands: []string{
				"echo '1\n2\n3'",
				"printf '\\033[2G'",
				"printf '\\033[L'",
				"printf '\\033[M'",
				"echo -n 'new'",
			},
			want: "1\nnew\n3",
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

func TestLineScrolling(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Scroll Region Up",
			commands: []string{
				"echo '1\n2\n3\n4'",
				"printf '\\033[2;3r'",
				"printf '\\033[2H'",
				"printf '\\033[S'",
				"printf '\\033[r'",
			},
			want: "1\n3\n4",
		},
		{
			name: "Scroll Region Down",
			commands: []string{
				"echo '1\n2\n3\n4'",
				"printf '\\033[2;3r'",
				"printf '\\033[2H'",
				"printf '\\033[T'",
				"printf '\\033[r'",
			},
			want: "1\n\n2\n4",
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

func TestLineWrappingModes(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Auto Wrap Mode",
			commands: []string{
				"printf '\\033[?7h'",
				"echo -n 'This is a very long line that should wrap automatically to the next line'",
			},
			want: "next line",
		},
		{
			name: "No Wrap Mode",
			commands: []string{
				"printf '\\033[?7l'",
				"echo -n 'This line should not wrap but truncate instead'",
				"printf '\\033[?7h'",
			},
			want: "truncate",
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
