# Windows Exit Code Debugging

## History of Changes

### Initial Implementation
- Used `syscall.WaitStatus` to get exit codes (Unix-style approach)
- All commands were wrapped with `cmd.exe /c`
- Exit codes were inconsistent, especially for internal commands

### First Fix Attempt (4308e66)
- Simplified Windows shell implementation
- Removed special handling for PowerShell
- Always used `cmd.exe /c` for command execution
- Still had issues with exit codes being 1 for successful commands

### Second Fix Attempt (ea41e48)
- Changed `GetExitCode()` to return `int` instead of `uint32`
- Used `syscall.WaitStatus` for exit code extraction
- Added graceful shutdown sequence (SIGINT → SIGTERM → Kill)
- Added test cases for exit codes, but they weren't Windows-specific

### Latest Changes
1. Simplified `GetExitCode()` to use `ProcessState.ExitCode()` directly
2. Added command type detection:
   ```go
   if isInternalCommand(command) {
       cmd = exec.Command("cmd.exe", "/c", command)
   } else {
       cmd = exec.Command(command)
   }
   ```
3. Added list of known internal commands for proper detection

## Current Understanding

### Windows Command Types
1. **Internal Commands** (`dir`, `echo`, `cd`, etc.)
   - Built into `cmd.exe`
   - Must be executed through `cmd.exe /c`
   - Exit codes are managed by `cmd.exe`

2. **External Commands** (`whoami`, `ping`, etc.)
   - Standalone executables
   - Can be executed directly
   - Return their own exit codes

### Exit Code Behavior
1. **cmd.exe /c behavior**:
   - Returns its own exit code, not necessarily the command's
   - Success = 0
   - Failure = 1 (even for some successful internal commands)
   - Can mask actual command exit codes

2. **Direct execution**:
   - Returns actual process exit code
   - More reliable for external commands
   - Not possible for internal commands

### Known Issues
1. Internal commands through `cmd.exe /c` may return 1 even on success
2. Complex commands (pipes, &&, ||) inherit exit code behavior from `cmd.exe`
3. No proper test coverage on actual Windows systems

## Next Steps

1. **Testing Needed**:
   - Validate exit codes for internal commands
   - Test complex command chains
   - Verify behavior in Windows CI environment

2. **Potential Solutions**:
   - Use `cmd.exe /c` only for internal commands
   - Parse `ERRORLEVEL` explicitly
   - Consider using PowerShell for better exit code handling

3. **Open Questions**:
   - How does `cmd.exe` handle exit codes for piped commands?
   - What's the proper way to handle complex command chains?
   - Should we switch to PowerShell as default shell? 