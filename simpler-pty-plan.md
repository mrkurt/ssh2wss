# Trick the Local Client with a Fake PTY

## **Overview**
This document outlines a plan to simplify PTY handling in FlySsh by **using a fake local PTY** to trick the client into thinking it’s talking to a real terminal, while forwarding all data to the **real** PTY on the remote server. This allows the session connection to remain raw while still supporting interactive programs like `vim`, `htop`, and `tmux`.

---

## **Why We Need a Fake Local PTY**
A terminal emulator (like `gnome-terminal`, `iTerm`, or `cmd.exe`) expects to communicate with a **real** PTY. If we just stream raw bytes from the remote PTY to the local terminal, we hit problems:

❌ **Programs like `vim` or `htop` break** because they expect a TTY.
❌ **Line wrapping and input behavior is inconsistent** since there’s no real PTY.
❌ **No way to properly handle window resizing** without a PTY.

Instead of running a **real shell locally**, we create a **fake PTY** that simply forwards everything to the remote PTY.

---

## **How It Works**
1. **Client opens a local PTY** (fake terminal device)
2. **Client forwards local PTY input to the remote PTY**
3. **Client forwards remote PTY output back to the local PTY**
4. **The local terminal emulator is none the wiser**—it thinks it’s talking to a real shell.

---

## **Implementation Plan**
### **1. Creating a Fake PTY on Unix**
On Unix-based systems, we use the `pty.Open()` function to create a local PTY. This acts as the interface between the user's terminal and the remote PTY.

```go
// Open a fake local PTY
localPTY, localTTY, err := pty.Open()
if err != nil {
    log.Fatal(err)
}

// Open WebSocket connection to remote PTY
remoteConn := openWebSocketToServer()

// Forward local PTY input to the remote PTY
go func() {
    io.Copy(remoteConn, localPTY) // Local input -> Remote PTY
}()

// Forward remote PTY output to the local PTY
go func() {
    io.Copy(localPTY, remoteConn) // Remote PTY -> Local PTY
}()
```

✅ **Client thinks it’s talking to a real PTY**
✅ **Remote PTY handles all terminal behaviors**
✅ **Works with interactive programs (`vim`, `htop`, `tmux`)**

---

### **2. Handling Window Resizes via the Fake PTY**
A real PTY would automatically handle `SIGWINCH` (window resize signals). Since we’re forwarding everything to the remote PTY, we need to ensure that resizing events propagate properly.

#### **Client: Detect Terminal Resize and Update the Remote PTY**
```javascript
window.addEventListener("resize", function () {
  let cols = process.stdout.columns;
  let rows = process.stdout.rows;

  controlSocket.send(JSON.stringify({
    type: "resize",
    cols: cols,
    rows: rows
  }));
});
```

#### **Server: Apply the Resize to the Remote PTY**
```go
ws.On("resize", func(msg ResizeMessage) {
    winsize := &pty.Winsize{
        Cols: uint16(msg.Cols),
        Rows: uint16(msg.Rows),
    }
    err := pty.Setsize(remotePTY, winsize)
    if err != nil {
        log.Println("Failed to resize PTY:", err)
    }
});
```

✅ **Client-side terminal adjusts normally**
✅ **Remote PTY resizes properly**
✅ **Maintains a clean protocol separation (session = raw, control = resize updates)**

---

### **3. Supporting Windows (Using ConPTY)**
Windows does not have Unix-style PTYs, but Windows 10+ includes **ConPTY** (Console Pseudo Terminal), which can be used to create a fake local PTY.

```javascript
const { spawn } = require('child_process');

const ptyProcess = spawn("cmd.exe", [], {
  windowsHide: true,
  stdio: "pipe"
});

ptyProcess.stdout.on("data", (data) => {
  socket.send(data.toString()); // Send to remote PTY
});

socket.on("message", (data) => {
  ptyProcess.stdin.write(data); // Receive from remote PTY
});
```

✅ **Windows users get a real PTY without needing Unix-like `/dev/pts/N`**
✅ **Remote PTY remains the only real shell**
✅ **Fully cross-platform implementation**

---

## **Final Architecture**
| **Component**     | **Action** |
|------------------|------------------------------------|
| **Local PTY**     | Fake PTY that forwards everything to remote |
| **Remote PTY**    | The real PTY that runs the shell/program |
| **Session Conn.** | Raw data stream (stdin/stdout forwarding) |
| **Control Conn.** | Resize events only |

✅ **Terminal emulator sees a PTY and works normally**
✅ **Session connection stays raw, no control messages mixed in**
✅ **Cross-platform: Works on Unix (PTY) and Windows (ConPTY)**

---

## **Next Steps**
1. **Implement local PTY forwarding (Unix & Windows)**
2. **Ensure resize events sync properly over control connection**
3. **Test with `vim`, `htop`, `tmux` to confirm interactive compatibility**

This approach keeps the protocol simple while ensuring full PTY behavior for interactive applications. 🚀

