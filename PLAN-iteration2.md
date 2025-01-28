# FlySSH Interactive Terminal Support

## Goal
Make `flyssh` work as a drop-in SSH replacement with a full interactive terminal that feels native and responsive.

## Next Iteration: Complete Terminal Support

### Objective
Implement full terminal support that matches native SSH client behavior:
- Proper terminal modes (raw, canonical, etc)
- Window resizing that works with all tools
- Signal handling (Ctrl+C, Ctrl+Z, etc)
- Color and cursor support
- Line editing (arrows, backspace, etc)

### Implementation Steps

1. Terminal Modes
```go
// Set comprehensive terminal modes
modes := ssh.TerminalModes{
    ssh.ECHO:          1,     // Display what we type
    ssh.ICANON:        1,     // Enable line editing
    ssh.ISIG:          1,     // Enable signals
    ssh.ICRNL:         1,     // Map CR to NL
    ssh.IEXTEN:        1,     // Enable extended input processing
    ssh.OPOST:         1,     // Enable output processing
    ssh.BRKINT:        1,     // Break generates interrupt
    ssh.IGNPAR:        1,     // Ignore parity errors
    ssh.INPCK:         0,     // No input parity check
    ssh.ISTRIP:        0,     // Don't strip high bit
    ssh.IXON:          1,     // Enable XON/XOFF flow control
    ssh.CS8:           1,     // 8 bits
    ssh.HUPCL:         1,     // Hang up on close
}
```

2. Window Handling
- Add TIOCGWINSZ support
- Handle dynamic window changes
- Update remote PTY size
- Test with split panes/tmux

3. Signal Support
- Map all standard signals:
  - SIGINT (Ctrl+C)
  - SIGTSTP (Ctrl+Z)
  - SIGQUIT (Ctrl+\)
- Handle window resize signals
- Clean shutdown on SIGHUP

4. Input/Output
- Raw mode handling
- Color escape sequences
- Cursor movement
- Line editing
- History support

### Success Criteria
1. All common terminal tools work:
   - vim/nano with proper display
   - tmux/screen with splits
   - top/htop with updates
   - bash with line editing

2. Terminal behavior matches OpenSSH:
   - Window resizing works
   - Colors display correctly
   - Ctrl keys work properly
   - Clean exit on signals

3. No regressions:
   - Cleanup still works
   - Error handling intact
   - Tests pass

### Testing
1. Unit Tests:
   - Terminal mode setting
   - Window size handling
   - Signal routing
   - Input/output processing

2. Integration Tests:
   - Full terminal session
   - Tool compatibility
   - Signal handling
   - Cleanup paths

3. Manual Test Matrix:
   ```
   | Feature          | vim/nano | tmux/screen | shell  | top/htop |
   |-----------------|----------|-------------|---------|----------|
   | Window Resize   |          |             |         |          |
   | Colors          |          |             |         |          |
   | Cursor Keys     |          |             |         |          |
   | Ctrl Keys       |          |             |         |          |
   | Line Editing    |          |             |         |          |
   | Split Panes     |          |             |         |          |
   | Clean Exit      |          |             |         |          |
   ```

### Implementation Order
1. Terminal modes and raw handling
2. Window resize support
3. Signal handling
4. Input/output processing
5. Manual testing and fixes

This iteration should be done as one complete unit since the features are interdependent:
- Terminal modes affect signal handling
- Window handling needs proper terminal setup
- Input processing depends on modes
- All features need proper cleanup

## Code Structure

### `client/session.go`