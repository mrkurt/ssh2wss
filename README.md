# SSH to WebSocket Bridge

A Go application that creates a bridge between SSH connections and WebSocket connections. It allows you to expose a WebSocket backend server through an SSH interface, enabling SSH clients to interact with WebSocket services.

## Features

- SSH server that accepts standard SSH connections
- Forwards SSH protocol data to a WebSocket server
- Supports command execution (`ssh host command`)
- Handles interactive sessions (`ssh host`)
- No client authentication required (configurable)
- Clean connection handling and error management

## Setup

1. Generate an SSH host key:
```bash
ssh-keygen -t rsa -f host.key -N ""
```

2. Run the bridge:
```bash
go run main.go --ssh-port 2222 --ws-endpoint ws://localhost:8080
```

Options:
- `--ssh-port`: Port to listen for SSH connections (default: 2222)
- `--ws-endpoint`: WebSocket server endpoint (default: ws://localhost:8080)
- `--key`: Path to SSH host key file (default: host.key)

## WebSocket Protocol Implementation Guide

To implement a compatible WebSocket server, follow these specifications:

### Message Format

The WebSocket server receives raw command data from SSH clients:

1. For command execution (e.g., `ssh host echo hello`):
   - Receives: The raw command string (e.g., "echo hello")
   - Should respond: Command output as a string with a trailing newline
   - Should close the connection after sending the response

2. For interactive sessions:
   - Receives: Raw input data from the SSH client
   - Should respond: Output data to be displayed to the SSH client
   - Should maintain an open connection until the session ends

### Server Requirements

1. Must maintain a persistent WebSocket connection for each SSH session
2. Must handle binary data (SSH protocol messages)
3. Must send appropriate responses for commands
4. Must handle connection closure properly

### Example Server Implementation

Simple command execution server:

```go
package main

import (
    "net/http"
    "strings"
    "golang.org/x/net/websocket"
)

func handleSSH(ws *websocket.Conn) {
    // Buffer for receiving commands
    var buf [1024]byte
    
    // Read the command
    n, err := ws.Read(buf[:])
    if err != nil {
        return
    }
    
    // Parse the command
    cmd := string(buf[:n])
    
    // Handle echo command
    if strings.HasPrefix(cmd, "echo") {
        // Extract the echo argument
        response := strings.TrimPrefix(cmd, "echo ")
        response = strings.TrimSpace(response)
        
        // Send response with newline
        ws.Write([]byte(response + "\n"))
    }
    
    // Close the connection after handling the command
    ws.Close()
}

func main() {
    http.Handle("/", websocket.Handler(handleSSH))
    http.ListenAndServe(":8080", nil)
}
```

## Testing

Run the tests using:
```bash
go test -v
```

The test suite includes:
- WebSocket server implementation
- SSH client connection testing
- Command execution verification
- Error handling verification

## Example Usage

1. Start the bridge:
```bash
go run main.go
```

2. Use SSH to execute commands:
```bash
# Execute a command
ssh -p 2222 localhost echo "hello world"

# Start an interactive session
ssh -p 2222 localhost
```

## Security Considerations

1. The bridge currently accepts any SSH client connection (no authentication)
2. All traffic between SSH client and WebSocket server is forwarded as-is
3. Use TLS for the WebSocket connection in production
4. Consider adding client authentication if needed 