package terminal

import (
	"bytes"
	"testing"
)

func TestScreenOperations(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{
			name: "Clear Screen",
			commands: []string{
				"echo 'old content'",
				"printf '\\033[2J'",
				"echo -n 'new content'",
			},
			want: "new content",
		},
		{
			name: "Clear Line",
			commands: []string{
				"echo -n 'old text'",
				"printf '\\033[K'",
				"echo -n 'new text'",
			},
			want: "new text",
		},
		{
			name: "Clear From Cursor To End",
			commands: []string{
				"echo -n 'keep this|remove this'",
				"printf '\\033[7G\\033[K'",
				"echo -n 'new end'",
			},
			want: "keep thnew end",
		},
		{
			name: "Clear From Cursor To Start",
			commands: []string{
				"echo -n 'remove this|keep this'",
				"printf '\\033[12G\\033[1K'",
				"echo -n 'new start|'",
			},
			want: "new start|keep this",
		},
		{
			name: "Scroll Up",
			commands: []string{
				"echo '1\n2\n3'",
				"printf '\\033[S'",
				"echo '4'",
			},
			want: "2\n3\n4",
		},
		{
			name: "Scroll Down",
			commands: []string{
				"echo '1\n2\n3'",
				"printf '\\033[T'",
				"printf '\\033[H'",
				"echo -n 'new top'",
			},
			want: "new top\n1\n2",
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
