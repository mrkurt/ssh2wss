# VSCode Remote Development Protocol

This document describes the protocol used for remote development in VSCode over WebSocket connections.

## Overview

The protocol enables VSCode to communicate with a remote server for:
- File operations
- Terminal management
- Process execution
- Extension host communication

## Connection Establishment

1. VSCode initiates a WebSocket connection to your server
2. Server accepts and performs any necessary authentication
3. Initial handshake occurs to establish capabilities

### Handshake Message
```json
{
  "type": "request",
  "id": 1,
  "method": "handshake",
  "params": {
    "capabilities": {
      "remoteShell": true,
      "fileSystem": true,
      "process": true,
      "terminal": true
    }
  }
}
```

## Message Format

All messages follow JSON-RPC style format:

```typescript
interface Message {
  type: 'request' | 'response' | 'event';
  id?: number;         // Required for requests/responses
  method: string;      // The operation to perform
  params?: any;        // Operation-specific parameters
  error?: {            // For error responses
    code: number;
    message: string;
    data?: any;
  };
  result?: any;        // For successful responses
}
```

## File Operations

### Read File
```json
// Request
{
  "type": "request",
  "id": 2,
  "method": "fs/read",
  "params": {
    "path": "/path/to/file",
    "encoding": "utf8"
  }
}

// Response
{
  "type": "response",
  "id": 2,
  "result": {
    "content": "file contents here"
  }
}
```

### Write File
```json
// Request
{
  "type": "request",
  "id": 3,
  "method": "fs/write",
  "params": {
    "path": "/path/to/file",
    "content": "new content",
    "options": {
      "create": true,
      "overwrite": true
    }
  }
}

// Response
{
  "type": "response",
  "id": 3,
  "result": true
}
```

### Delete File
```json
// Request
{
  "type": "request",
  "id": 4,
  "method": "fs/delete",
  "params": {
    "path": "/path/to/file",
    "recursive": false
  }
}
```

### File Stats
```json
// Request
{
  "type": "request",
  "id": 5,
  "method": "fs/stat",
  "params": {
    "path": "/path/to/file"
  }
}

// Response
{
  "type": "response",
  "id": 5,
  "result": {
    "type": "file",
    "size": 1234,
    "mtime": 1234567890,
    "ctime": 1234567890,
    "permissions": "644"
  }
}
```

## Terminal Operations

### Create Terminal
```json
// Request
{
  "type": "request",
  "id": 6,
  "method": "terminal/create",
  "params": {
    "cols": 80,
    "rows": 24,
    "env": {
      "TERM": "xterm-256color"
    }
  }
}

// Response
{
  "type": "response",
  "id": 6,
  "result": {
    "terminalId": "term1"
  }
}
```

### Terminal Input
```json
// Request
{
  "type": "request",
  "id": 7,
  "method": "terminal/input",
  "params": {
    "terminalId": "term1",
    "data": "ls -la\n"
  }
}
```

### Terminal Output (Server to Client)
```json
// Event
{
  "type": "event",
  "method": "terminal/output",
  "params": {
    "terminalId": "term1",
    "data": "total 12\ndrwxr-xr-x ..."
  }
}
```

### Resize Terminal
```json
// Request
{
  "type": "request",
  "id": 8,
  "method": "terminal/resize",
  "params": {
    "terminalId": "term1",
    "cols": 100,
    "rows": 30
  }
}
```

## Process Operations

### Execute Process
```json
// Request
{
  "type": "request",
  "id": 9,
  "method": "process/exec",
  "params": {
    "command": "git",
    "args": ["status"],
    "cwd": "/workspace",
    "env": {
      "PATH": "/usr/local/bin:/usr/bin"
    }
  }
}

// Response
{
  "type": "response",
  "id": 9,
  "result": {
    "exitCode": 0,
    "stdout": "On branch main...",
    "stderr": ""
  }
}
```

## Implementation Notes

1. Error Handling
```json
// Error Response Example
{
  "type": "response",
  "id": 10,
  "error": {
    "code": 404,
    "message": "File not found",
    "data": {
      "path": "/nonexistent/file"
    }
  }
}
```

2. Authentication
- Use standard WebSocket authentication mechanisms (headers, tokens)
- Initial handshake should include any necessary credentials

3. Binary Data
- Use base64 encoding for binary file content
- Or use WebSocket binary frames for better performance

## Minimal Implementation

For a basic working implementation, you need to support:
1. File operations (read/write/delete)
2. Terminal creation and I/O
3. Basic process execution

Example minimal server implementation:
```typescript
interface RemoteServer {
  // File operations
  async readFile(path: string): Promise<Buffer>;
  async writeFile(path: string, content: Buffer): Promise<void>;
  async deleteFile(path: string): Promise<void>;
  
  // Terminal operations
  async createTerminal(cols: number, rows: number): Promise<string>;
  async writeTerminal(id: string, data: string): Promise<void>;
  async resizeTerminal(id: string, cols: number, rows: number): Promise<void>;
  
  // Process operations
  async executeCommand(command: string, args: string[]): Promise<{
    exitCode: number;
    stdout: string;
    stderr: string;
  }>;
}
```

## VSCode Integration

To use this protocol in a VSCode extension:

1. Register your custom protocol:
```typescript
vscode.workspace.registerRemoteAuthorityResolver('my-remote', {
  resolve(authority: string): Thenable<vscode.ResolverResult> {
    return {
      authority: authority,
      socketFactory: {
        connect: async () => {
          const socket = new WebSocket(`wss://${authority}`);
          // Implement protocol handling here
          return socket;
        }
      }
    };
  }
});
```

2. Connect using:
```typescript
await vscode.commands.executeCommand(
  'vscode.openFolder',
  vscode.Uri.parse('my-remote://server-address/workspace/path')
);
```

## Security Considerations

1. Always use WSS (WebSocket Secure)
2. Implement proper authentication
3. Validate all file paths
4. Sanitize command inputs
5. Implement proper error handling
6. Consider rate limiting
7. Implement proper logging

## Testing

Test scenarios should cover:
1. File operations with various encodings
2. Terminal operations with different outputs
3. Process execution with different commands
4. Error conditions and recovery
5. Connection loss and reconnection
6. Authentication and security 