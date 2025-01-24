# Windows Timeout and Handle Debugging

## History of Changes

### Initial Issues
- Interactive shell tests failing with "context deadline exceeded"
- Goroutine leaks detected in tests (2 → 4 goroutines)
- SSH and WebSocket servers not shutting down properly
- Handle leaks in Windows process management

### First Fix Attempt (453a305)
- Added graceful shutdown with context
- Skipped Windows interactive shell test temporarily
- Added proper cleanup in test teardown
- Implemented context-based server shutdown

### Latest Changes
- Explicitly skipped all interactive shell tests on Windows
- Added clear documentation about PTY support being required
- Removed attempts to run interactive tests in CI
- Focused on non-interactive command execution for Windows

### Server Shutdown Changes
1. Bridge Implementation:
   ```go
   type Bridge struct {
       wsServer  *WebSocketServer
       sshServer *SSHServer
       ctx       context.Context
       cancel    context.CancelFunc
   }
   ```

2. WebSocket Server:
   - Added context to `Start()` method
   - Implemented graceful shutdown using `http.Server`
   - Added cleanup for WebSocket connections

3. SSH Server:
   - Added context to `Start()` method
   - Implemented shutdown channel for clean exit
   - Added proper handle cleanup for Windows

### Handle Management
1. **Process Handles**:
   - Ensured proper handle closure in `Shell.Close()`
   - Added cleanup in process termination
   - Implemented graceful shutdown sequence

2. **Pipe Handles**:
   - Added proper cleanup for stdin/stdout/stderr pipes
   - Ensured handles are closed in error cases
   - Implemented synchronization for pipe closure

### Test Improvements
1. **Goroutine Leak Detection**:
   ```go
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
   ```

2. **Test Cleanup**:
   - Added `t.Cleanup()` for proper teardown
   - Implemented context cancellation
   - Added sleep to allow for graceful shutdown

### Known Issues
1. Interactive Shell Support
   - All interactive shell tests now skipped on Windows
   - PTY/TTY support not implemented yet
   - Will require significant Windows console implementation
   - Tracking issue for future implementation

2. Resource Cleanup
   - Some handles may not be properly closed
   - Need better synchronization for cleanup
   - Potential race conditions in shutdown

3. Test Environment
   - CI environment behaving differently than local
   - Timeout values may need adjustment
   - Debug logging affecting buffer handling

## Next Steps

1. **Testing Needed**:
   - Validate handle cleanup on Windows
   - Test server shutdown sequences
   - Monitor handle usage in CI environment

2. **Potential Solutions**:
   - Research Windows PTY/console implementation options
   - Add handle tracking and verification
   - Improve shutdown synchronization

3. **Open Questions**:
   - What's the best approach for Windows PTY support?
   - Are there existing Go packages for Windows console?
   - How to properly implement interactive shells on Windows? 