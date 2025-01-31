package core

import (
	"io"
)

// TerminalTransform represents a transformation of terminal I/O
type TerminalTransform interface {
	// Transform wraps the given reader/writer with the transformation
	Transform(in io.Reader, out io.Writer) (io.Reader, io.Writer, error)

	// Cleanup performs any necessary cleanup when done
	Cleanup() error
}

// WindowSizeHandler represents something that can detect and send window size changes
type WindowSizeHandler interface {
	// StartMonitoring begins monitoring for window size changes
	// It calls the callback with new dimensions when they change
	StartMonitoring(callback func(width, height uint16)) error

	// StopMonitoring stops monitoring for window size changes
	StopMonitoring() error
}

// TerminalMode represents a terminal mode transformation (raw mode, etc)
type TerminalMode interface {
	TerminalTransform
	WindowSizeHandler
}
