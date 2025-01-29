# FlySSH Implementation Plan

## ✅ SSH-over-WebSocket (Done)
- Proof of concept working with test
- Basic command execution working (`flyssh -f "command"`)
- Simplified CLI with just the essentials (-s, -t, -f flags)
- Client mode as the default

## Interactive Mode
Goal: Make `flyssh` work as a drop-in SSH replacement with a full interactive terminal.

What works now:
- Basic SSH-over-WebSocket connection
- Simple command execution
- Environment variable configuration

What we need:
1. Full Terminal Support
   - Proper terminal dimensions
   - Window resize handling
   - Color and formatting support
   - Keyboard input handling (arrows, ctrl keys, etc)

2. Clean Session Management
   - Handle Ctrl+C gracefully
   - Clean up resources on exit
   - Restore terminal state after crashes
   - Support session reconnection

3. Error Handling & Feedback
   - Clear error messages for connection issues
   - Status indicators for connection state
   - Helpful suggestions for common problems

Implementation:
- Add terminal dimension detection to `internal/client/client.go`
- Add SIGWINCH handler for window resizing
- Improve terminal state management in `internal/client/terminal.go`
- Add session cleanup in `internal/client/session.go`
- Add connection status handling in `internal/client/connection.go`

## Future Work
- Token validation and security improvements
- Connection retries and reconnection
- Better environment variable handling 