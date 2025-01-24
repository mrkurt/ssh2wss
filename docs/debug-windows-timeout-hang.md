# Windows Timeout and Handle Debugging

## Grep Patterns for Debugging

To filter relevant shutdown logs, use:
```
grep -E "(Bridge Stop|context canceled|shutdown|listener closed|Failed to accept)"
```

This pattern will show:
- When Bridge.Stop() is called
- Context cancellation for both servers
- Server shutdown progress
- Listener closure
- Any failed connection attempts during shutdown

Example of clean shutdown sequence:
```
Bridge Stop() called, canceling context
Bridge context canceled, waiting for servers to shut down
WebSocket server context canceled, shutting down
WebSocket server shutdown complete
SSH server context canceled, closing listener
SSH server listener closed
Bridge shutdown complete
```

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
- Implemented proper shutdown signaling for both servers
- Added completion channels to track server shutdown
- Fixed race conditions in shutdown sequence
- Improved Windows command execution and exit code handling
- Added timeout context for WebSocket server shutdown

### Server Shutdown Changes
1. Bridge Implementation:
   ```go
   type Bridge struct {
       wsServer  *WebSocketServer
       sshServer *SSHServer
       ctx       context.Context
       cancel    context.CancelFunc
       sshDone   chan struct{} // signals SSH server shutdown complete
       wsDone    chan struct{} // signals WebSocket server shutdown complete
   }
   ```

2. WebSocket Server:
   - Added context to `Start()` method
   - Implemented graceful shutdown using `http.Server`
   - Added cleanup for WebSocket connections
   - Added 5-second timeout for graceful shutdown
   - Added shutdown completion signaling

3. SSH Server:
   - Added context to `Start()` method
   - Implemented shutdown and completion channels
   - Added proper handle cleanup for Windows
   - Improved connection acceptance handling during shutdown
   - Fixed race conditions in listener closure

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
   - Added proper shutdown sequence verification
   - Fixed timing issues in shutdown tests

### Known Issues
1. Interactive Shell Support
   - All interactive shell tests now skipped on Windows
   - PTY/TTY support not implemented yet
   - Will require significant Windows console implementation
   - Tracking issue for future implementation

2. Resource Cleanup
   - Improved handle cleanup but may need further testing
   - Added better synchronization for cleanup
   - Fixed most race conditions in shutdown

3. Test Environment
   - CI environment behaving differently than local
   - Exit code handling differs between Windows and macOS
   - Added more debug logging for shutdown sequence

## Next Steps

1. **Testing Needed**:
   - Further validation of handle cleanup on Windows
   - More extensive testing of server shutdown sequences
   - Monitor handle usage in CI environment
   - Test exit code handling across different platforms

2. **Potential Solutions**:
   - Research Windows PTY/console implementation options
   - Add handle tracking and verification
   - Consider platform-specific exit code handling

3. **Open Questions**:
   - What's the best approach for Windows PTY support?
   - Are there existing Go packages for Windows console?
   - How to properly implement interactive shells on Windows?
   - Should exit code handling be platform-specific? 