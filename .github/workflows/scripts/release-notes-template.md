# flyssh: WebSocket SSH Terminal

A lightweight SSH-over-WebSocket implementation that enables SSH access through WebSocket connections.

## Core Features

### Terminal Support
- Full PTY support with window resizing
- ANSI/color support
- Interactive command execution

### Security
- Token-based authentication
- End-to-end encryption
- Standard SSH protocol

### Client Compatibility
- Works with standard SSH clients
- No client modifications needed
- Native SSH experience

### Implementation Details
- Built with Go's stdlib crypto/ssh package
- Clean, modular architecture
- Comprehensive test coverage

## Architecture
1. Core package - shared functionality
2. Client - WebSocket SSH client
3. Server - WebSocket SSH server

This release provides a solid foundation for SSH-over-WebSocket communication with a focus on simplicity and security. 