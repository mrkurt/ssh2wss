# FlySSH Client Mode Implementation Plan

## Current State
- Basic client package in `internal/client/` with core SSH functionality
- Support for direct SSH and proxy modes
- Basic WebSocket connection handling
- Terminal handling and restoration
- Basic I/O handling
- Incomplete command-line interface in `cmd/flyssh/commands/client.go`

## Issues to Address
1. Inconsistent command structure between `commands/client.go` and `main.go`
2. Client mode mixed with exec mode
3. Unvalidated environment variable usage
4. Incomplete command-line flag structure

## Implementation Plan

### Iteration 1: Command Structure Cleanup
1. Consolidate command handling into `cmd/flyssh/commands/`
2. Separate exec mode from client mode
3. Implement proper client command flags:
   ```
   flyssh client [options] <destination>
   Options:
     -i, --identity     SSH identity file
     -p, --port        Port number (default: 22)
     -u, --user        Username
     -s, --server      WebSocket server URL
     -t, --token       Auth token
     -c, --command     Command to execute (non-interactive mode)
   ```

### Iteration 2: Client Implementation
1. Enhance `internal/client/client.go`:
   - Proper SSH key handling
   - Interactive vs non-interactive mode support
   - Enhanced error handling
   - Complete session management
   - Improved I/O handling

### Iteration 3: Authentication & Security
1. Token validation implementation
2. SSH key management
3. Connection security checks
4. Environment variable security improvements

## Success Criteria
- Clean command-line interface
- Stable SSH connections
- Proper error handling and user feedback
- Secure authentication and key management
- Support for both interactive and non-interactive modes 