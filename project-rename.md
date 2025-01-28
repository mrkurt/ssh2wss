# Project Rename: flyssh → flyssh

## Overview
We are renaming this project from `flyssh` to `flyssh` to better reflect its purpose as Fly.io's tool for connecting to remote development environments in Machines.

## Implementation Steps

### 1. Code Changes

#### Package Names
```bash
# Update go.mod
module flyssh

# Update imports in all .go files from
import "flyssh/server"
# to
import "flyssh/server"
```

#### Binary Names
- Rename main binary from `flyssh` to `flyssh`
- Rename client binary from `flyssh-client` to `flyssh-client`
- Update any build scripts or Makefile

#### Configuration
- Update environment variable prefixes:
  - `FLYSSH_SERVER` → `FLYSSH_SERVER`
  - `FLYSSH_AUTH_TOKEN` → `FLYSSH_AUTH_TOKEN`
- Update config file locations:
  - `~/.flyssh/` → `~/.fly/ssh/`
  - `/etc/flyssh/` → `/etc/fly/ssh/`

### 2. Documentation Updates

#### README.md
- Update project name and description
- Update command examples
- Update configuration examples
- Update installation instructions

#### Other Documentation
- Update code comments
- Update markdown files

### 3. VSCode Integration
- Update configuration keys:
  ```jsonc
  {
    "flyssh.server": "wss://example.com:8081",
    "flyssh.authToken": "your-token"
  }
  ```

### 4. Testing

1. **Functionality Testing**
   - All commands work with new binary names
   - Configuration loading works
   - VSCode integration works

2. **Platform Testing**
   - Test on Linux
   - Test on macOS
   - Test on Windows

## Implementation Commands

```bash
# 1. Rename module
sed -i '' 's/module flyssh/module flyssh/' go.mod

# 2. Update all Go imports
find . -type f -name "*.go" -exec sed -i '' 's/flyssh\//flyssh\//g' {} +

# 3. Update environment variables
find . -type f -exec sed -i '' 's/FLYSSH_/FLYSSH_/g' {} +

# 4. Update config paths
find . -type f -exec sed -i '' 's/\.flyssh/\.fly\/ssh/g' {} +
find . -type f -exec sed -i '' 's/etc\/flyssh/etc\/fly\/ssh/g' {} +

# 5. Update binary names in documentation and scripts
find . -type f -name "*.md" -exec sed -i '' 's/flyssh-client/flyssh-client/g' {} +
find . -type f -name "*.md" -exec sed -i '' 's/flyssh/flyssh/g' {} +
```

## Verification Steps

1. Clean and rebuild:
```bash
go clean -cache
go build -o flyssh
```

2. Run tests:
```bash
go test ./...
```

3. Manual testing:
- Test basic SSH connection
- Test VSCode integration
- Verify environment variables
- Check config file loading 
