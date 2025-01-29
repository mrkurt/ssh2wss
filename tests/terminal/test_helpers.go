package terminal

import (
	"bytes"
	"regexp"
)

// Helper function to filter out server logs from output
func filterServerLogs(output []byte) []byte {
	lines := bytes.Split(output, []byte("\n"))
	var filtered [][]byte
	for _, line := range lines {
		if !bytes.Contains(line, []byte("SSH server")) &&
			!bytes.Contains(line, []byte("New SSH connection")) &&
			!bytes.Contains(line, []byte("PTY requested")) &&
			!bytes.Contains(line, []byte("Starting shell handler")) &&
			!bytes.Contains(line, []byte("Command finished")) &&
			!bytes.Contains(line, []byte("Shell handler completed")) &&
			!bytes.Contains(line, []byte("Executing command")) &&
			!bytes.Contains(line, []byte("Sending exit status")) {
			filtered = append(filtered, line)
		}
	}
	return bytes.TrimSpace(bytes.Join(filtered, []byte("\n")))
}

var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\].*?(?:\x1b\\|\x07)|\[[0-9;]*[mK]|\(B)`)

// stripANSI removes ANSI escape sequences from the output
func stripANSI(data []byte) []byte {
	return bytes.TrimSpace(ansiPattern.ReplaceAll(data, nil))
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
