# VSCode Integration Plan

## Current Approach: Drop-in SSH Client

### Overview
We will create a drop-in replacement for the SSH client that VSCode's Remote SSH extension uses. This client will connect directly to our WebSocket server without requiring a local SSH server or proxy.

### Implementation Details

#### 1. Client Binary (`flyssh-client`)
- Single binary that implements SSH client protocol
- Reads configuration from environment variables
- Direct WebSocket connection to remote server
- Full SSH protocol implementation including:
  - PTY support
  - Window resizing
  - Environment variables
  - Non-interactive command execution
  - Authentication

#### 2. Environment Configuration
The client will read configuration from environment variables:
```bash
FLYSSH_SERVER=wss://example.com:8081
FLYSSH_AUTH_TOKEN=your-secret-token
```

#### 3. Command-Line Interface
```bash
flyssh-client [standard-ssh-flags] [host]
```
- Must support all standard SSH flags that VSCode uses
- Must exit with appropriate status codes
- Must handle SIGTERM and other signals properly

#### 4. VSCode Configuration
```jsonc
{
  "remote.SSH.path": "/path/to/flyssh-client",
  "flyssh.server": "wss://example.com:8081",
  "flyssh.authToken": "your-token"
}
```

### Technical Requirements

#### SSH Protocol Implementation
1. **Authentication**
   - Support for password authentication
   - Support for public key authentication
   - Support for keyboard-interactive
   - Host key verification

2. **Terminal Support**
   - Full PTY implementation
   - Window size management
   - SIGWINCH handling
   - Color support
   - Control characters

3. **Channel Management**
   - Session channels
   - Direct TCP/IP forwarding
   - Local port forwarding
   - Remote port forwarding

4. **Data Transfer**
   - Efficient binary protocol
   - Compression support
   - Keep-alive mechanisms
   - Proper EOF handling

#### WebSocket Integration
1. **Connection**
   - TLS/SSL support
   - Proxy support
   - Authentication headers
   - Reconnection handling

2. **Protocol**
   - Binary message format
   - Frame size optimization
   - Error handling
   - Clean shutdown

### Development Plan

1. **Phase 1: Basic SSH Client**
   - Implement basic SSH client functionality
   - Support standard SSH flags
   - Basic terminal support
   - Environment variable configuration

2. **Phase 2: WebSocket Integration**
   - Add WebSocket connection layer
   - Implement authentication
   - Handle reconnection
   - Add TLS support

3. **Phase 3: Advanced Features**
   - Full PTY support
   - Window resizing
   - Port forwarding
   - Keep-alive

4. **Phase 4: VSCode Integration**
   - Test with VSCode Remote SSH
   - Handle all VSCode-specific requirements
   - Implement proper signal handling
   - Add debug logging

### Testing Strategy

1. **Unit Tests**
   - SSH protocol implementation
   - WebSocket handling
   - Configuration management
   - Signal handling

2. **Integration Tests**
   - Full connection flow
   - Terminal behavior
   - Authentication scenarios
   - Error conditions

3. **VSCode-specific Tests**
   - Remote SSH extension compatibility
   - Settings handling
   - Connection management
   - Error reporting

## Alternative Approaches Considered

### 1. Local SSH Server + ProxyCommand
**Description:**
Run a local SSH server that forwards connections through WebSocket.

**Pros:**
- Simpler SSH implementation
- More standard approach
- Easier to debug

**Cons:**
- Requires local port
- More complex setup
- System configuration changes
- Permission issues on some systems

### 2. Custom VSCode Extension Protocol
**Description:**
Implement a custom protocol instead of using SSH.

**Pros:**
- More control over the protocol
- Could be more efficient
- Simpler implementation

**Cons:**
- No SSH compatibility
- Would need custom extension
- More maintenance burden
- Less secure

### 3. WebSocket Proxy in Extension
**Description:**
Implement the WebSocket proxy in the VSCode extension itself.

**Pros:**
- No external binaries needed
- Easier distribution
- Better integration with VSCode

**Cons:**
- Limited by Node.js
- Performance concerns
- More complex extension
- Platform limitations

## Security Considerations

### 1. Authentication
- Token storage
- Key management
- Password handling
- Host verification

### 2. Encryption
- TLS for WebSocket
- SSH protocol encryption
- Key exchange
- Forward secrecy

### 3. Permissions
- File access
- Port binding
- Process creation
- Signal handling

## Future Enhancements

### 1. Performance Optimizations
- Connection pooling
- Binary protocol optimizations
- Compression tuning
- Memory management

### 2. Additional Features
- Multi-hop support
- Custom authentication methods
- Advanced port forwarding
- Protocol extensions

### 3. Developer Experience
- Debug logging
- Performance profiling
- Error reporting
- Configuration validation

## Known Limitations

1. **Platform Support**
   - Windows-specific challenges
   - Path handling differences
   - Signal handling variations
   - Terminal compatibility

2. **VSCode Integration**
   - Extension API limitations
   - Settings scope issues
   - Update management
   - Error handling constraints

3. **Protocol**
   - WebSocket overhead
   - Latency considerations
   - Reconnection edge cases
   - Proxy limitations

## Conclusion

The drop-in SSH client approach provides the best balance of:
- User experience (minimal configuration)
- Security (standard SSH + TLS)
- Maintainability (single binary)
- Compatibility (works with existing tools)

This approach allows us to maintain the security and familiarity of SSH while adding the flexibility of WebSocket transport, all without requiring users to modify their system configuration or understand the underlying complexity. 