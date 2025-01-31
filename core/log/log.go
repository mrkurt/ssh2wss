package log

import (
	"io"
	"log"
	"os"
)

var (
	// Info logger for normal operations
	Info = log.New(os.Stdout, "", log.LstdFlags)

	// Debug logger for detailed debugging
	Debug = log.New(io.Discard, "DEBUG: ", log.LstdFlags|log.Lmsgprefix)
)

// Init sets up loggers based on environment
func Init() {
	if os.Getenv("WSS_DEBUG") == "1" {
		Debug.SetOutput(os.Stderr)
	}
}
