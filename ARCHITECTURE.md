# FlySsh Architecture

FlySsh is a WebSocket-based terminal multiplexer that enables secure terminal sharing over standard web ports. The system consists of a client and server component that communicate over two WebSocket connections: one for terminal data and one for control messages.

## Connection Flow

When a client connects, it first establishes a data WebSocket connection to the server's root path ("/"). The server generates a unique session ID and sends it to the client. This session ID is used to coordinate terminal settings and state between the data and control channels.

After receiving the session ID, the client establishes a second WebSocket connection to the "/control" path. This control connection is used exclusively for window resize events and other terminal control messages that shouldn't be mixed with the data stream.

Both connections are authenticated using a shared token passed as a URL parameter. In development mode, this token is automatically generated and shared between the client and server processes.

## Terminal Handling

The server creates a new PTY (pseudo-terminal) for each client connection using the system's PTY allocation facilities (via the creack/pty package). The PTY is configured with a minimal environment that matches standard SSH server behavior: TERM=xterm, a basic PATH, and a simple shell prompt.

The server maintains a map of active PTYs indexed by session ID. This map is protected by sync.Map for concurrent access, as each client has multiple goroutines accessing its PTY (one for reading, one for writing).

Terminal resizing is handled through the control channel. When the client's terminal size changes (detected via SIGWINCH signals), it sends a resize message with the new dimensions over the control WebSocket. The server looks up the corresponding PTY by session ID and applies the new dimensions using the PTY's TIOCSWINSZ ioctl.

## Data Transfer

Terminal I/O uses the standard Go io.Copy pattern in both directions. The client reads from stdin and writes to the WebSocket, while simultaneously reading from the WebSocket and writing to stdout. The server does the same with the WebSocket and PTY.

This bidirectional copying happens in separate goroutines to prevent blocking. When either direction encounters an error or EOF, it signals completion through a channel. This matches the pattern used in Go's crypto/ssh package and other terminal handling code.

## Raw Mode

When the client detects it's running in a real terminal (vs being piped or redirected), it switches the terminal into raw mode. This disables local echo and line buffering, allowing character-by-character transmission and proper handling of control sequences. The original terminal state is restored when the client exits.

## Error Handling

The system distinguishes between normal connection closure (EOF, "use of closed network connection", WebSocket close 1000) and actual errors. This prevents logging noise when sessions end normally. Both client and server maintain their own error logging, with the client focusing on connection issues and the server focusing on PTY operations.

## Development Mode

In development mode, the client automatically starts a local server process and connects to it. It uses a random high port (49152-65535) to avoid conflicts with other services. The development token is generated using crypto/rand for security, even in local testing.

## Limitations

The current architecture assumes a Unix-like system with PTY support. While the WebSocket transport would work on any platform, the terminal handling is specifically designed around Unix PTY semantics. Windows support would require significant changes to use ConPTY or similar Windows-specific terminal APIs. 