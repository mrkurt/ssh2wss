package server

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"testing"
	"time"
)

// trackOperation logs the start and end of a potentially hanging operation
func trackOperation(name string) func() {
	start := time.Now()
	pc, file, line, _ := runtime.Caller(1)
	caller := runtime.FuncForPC(pc).Name()
	parts := strings.Split(caller, ".")
	funcName := parts[len(parts)-1]
	location := fmt.Sprintf("%s:%d", file, line)

	log.Printf("[TRACK] %s started in %s at %s (location: %s)", name, funcName, start.Format(time.RFC3339Nano), location)
	return func() {
		end := time.Now()
		duration := end.Sub(start)
		log.Printf("[TRACK] %s completed in %s after %v", name, funcName, duration)
	}
}

// checkGoroutineLeak detects goroutine leaks in tests
func checkGoroutineLeak(t *testing.T) func() {
	initialGoroutines := runtime.NumGoroutine()
	return func() {
		time.Sleep(100 * time.Millisecond)
		finalGoroutines := runtime.NumGoroutine()
		if finalGoroutines > initialGoroutines {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Errorf("Goroutine leak: had %d, now have %d\n%s",
				initialGoroutines, finalGoroutines, buf[:n])
		}
	}
}
