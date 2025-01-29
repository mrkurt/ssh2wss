# flyssh: SSH over WebSocket

A lightweight, secure command-line tool that tunnels SSH connections through WebSocket, enabling SSH access in environments where traditional SSH ports (22) are blocked or unavailable. Perfect for cloud environments, corporate networks, and other restricted environments.

## Key Features

### Core Functionality
- **SSH over WebSocket**: Tunnels SSH traffic through WebSocket connections (port 80/443)
- **Token Authentication**: Simple, secure token-based authentication for WebSocket connections
- **End-to-End Encryption**: All traffic is encrypted using SSH protocol
- **Cross-Platform**: Works on Linux, macOS, and other Unix-like systems

### Terminal Support
- **Full PTY Support**: Complete terminal emulation with all features:
  * Window resizing (automatic detection)
  * ANSI/Color support
  * Environment variables
  * Control sequences
- **Shell Compatibility**: Works with your default shell (bash, zsh, etc.)
- **Interactive & Non-Interactive**: Supports both interactive sessions and one-off commands

### Security Features
- **Token-Based Auth**: Simple but secure authentication for WebSocket connections
- **SSH Encryption**: All traffic is encrypted using standard SSH protocols
- **Automatic Key Management**: Host keys are automatically generated and managed
- **No Root Required**: Runs entirely in user space, no system modifications needed

## Quick Start

### Installation

```bash
# Using Go (1.22 or later)
go install github.com/superfly/flyssh/cmd/flyssh@latest

# Or build from source
git clone https://github.com/superfly/flyssh.git
cd flyssh
go build -o flyssh ./cmd/flyssh
```

### Basic Usage

1. Start the server:
```bash
# Generate a secure token
export WSS_AUTH_TOKEN=$(openssl rand -hex 16)
echo "Your auth token: $WSS_AUTH_TOKEN"

# Start the server
flyssh server
```

2. Connect from a client:
```bash
# Use the same token from the server
export WSS_AUTH_TOKEN=<token-from-server>

# Start an interactive session
flyssh client ws://server:8081

# Or run a single command
flyssh client ws://server:8081 -- uptime
```

## Detailed Usage

### Server Mode

The server component runs on the machine you want to access:

```bash
# Basic server with default settings
flyssh server

# Custom port and development mode
flyssh server -port 8082 -dev

# With debug logging
WSS_DEBUG=1 flyssh server
```

Server Options:
- `-port`: WebSocket port (default: 8081)
- `-dev`: Enable development mode with auto-generated token
- Environment Variables:
  * `WSS_AUTH_TOKEN`: Required authentication token
  * `WSS_DEBUG`: Enable debug logging
  * `SHELL`: Shell to use for sessions (default: system shell)

### Client Mode

The client connects to a running server:

```bash
# Interactive session
flyssh client -url ws://server:8081

# Run command and exit
flyssh client -url ws://server:8081 -- ls -la

# With custom token
flyssh client -url ws://server:8081 -token your-auth-token
```

Client Options:
- `-url`: WebSocket server URL (required)
- `-token`: Auth token (can also use WSS_AUTH_TOKEN env var)
- Environment Variables:
  * `WSS_AUTH_TOKEN`: Authentication token
  * `WSS_DEBUG`: Enable debug logging

## Architecture

The system uses a layered approach for security and compatibility:

1. **Transport Layer** (WebSocket)
   - Provides connectivity through standard web ports
   - Handles proxies and firewalls
   - Maintains persistent connections

2. **Security Layer** (Token Auth + SSH)
   - Token authentication for WebSocket connections
   - SSH protocol for end-to-end encryption
   - Automatic key management

3. **Terminal Layer** (PTY)
   - Full terminal emulation
   - Window size management
   - Signal handling
   - Shell session management

## Development

### Requirements
- Go 1.22 or later
- Unix-like system (Linux, macOS, etc.)

### Building
```bash
# Get dependencies
go mod download

# Build binary
go build -o flyssh ./cmd/flyssh

# Run tests
go test -v ./...
```

### Running Tests
```bash
# Run all tests
go test -v ./...

# Run specific test
go test -v ./tests -run TestClientBinary

# With debug output
WSS_DEBUG=1 go test -v ./...
```

## Troubleshooting

### Common Issues

1. Connection Refused
```bash
# Check server is running
ps aux | grep flyssh

# Verify port is open
nc -zv server 8081
```

2. Authentication Failed
```bash
# Check token is set
echo $WSS_AUTH_TOKEN

# Verify token matches server
# Server logs will show auth failures
```

3. Terminal Issues
```bash
# Reset terminal if it gets corrupted
reset

# Check terminal settings
stty -a
```

### Debug Mode

Enable debug logging for more information:
```bash
# Server debug mode
WSS_DEBUG=1 flyssh server

# Client debug mode
WSS_DEBUG=1 flyssh client -url ws://server:8081
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

MIT License - see LICENSE file for details