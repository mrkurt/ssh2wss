#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print with color
print_status() {
    echo -e "${GREEN}$1${NC}"
}

print_warning() {
    echo -e "${YELLOW}$1${NC}"
}

print_error() {
    echo -e "${RED}$1${NC}"
}

# Function to handle cleanup on error
cleanup() {
    print_error "Error occurred. Cleaning up..."
    git checkout main
    git branch -D "$TEMP_BRANCH" || true
    exit 1
}

# Set up error handling
trap cleanup ERR

# Get the root directory of the project
ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

# Create a temporary branch with timestamp
TIMESTAMP=$(date +%s)
TEMP_BRANCH="temp-rename-to-flyssh-$TIMESTAMP"
print_status "Creating temporary branch: $TEMP_BRANCH"
git checkout -b "$TEMP_BRANCH"

# Update go.mod
print_status "Updating go.mod..."
if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' 's|module ssh2wss|module flyssh|' go.mod
else
    sed -i 's|module ssh2wss|module flyssh|' go.mod
fi

# Update Go imports
print_status "Updating Go imports..."
while IFS= read -r -d '' file; do
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' 's|"ssh2wss/|"flyssh/|g' "$file"
    else
        sed -i 's|"ssh2wss/|"flyssh/|g' "$file"
    fi
done < <(find . -type f -name "*.go" -print0)

# Update environment variables and config paths
print_status "Updating environment variables and config paths..."
while IFS= read -r -d '' file; do
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' 's|SSH2WSS_|FLYSSH_|g' "$file"
    else
        sed -i 's|SSH2WSS_|FLYSSH_|g' "$file"
    fi
done < <(find . -type f \( -name "*.go" -o -name "*.md" \) -print0)

# Update binary names in documentation
print_status "Updating binary names in documentation..."
while IFS= read -r -d '' file; do
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' 's|ssh2wss|flyssh|g' "$file"
    else
        sed -i 's|ssh2wss|flyssh|g' "$file"
    fi
done < <(find . -type f -name "*.md" -print0)

# Clean Go cache and rebuild
print_status "Cleaning Go cache and rebuilding..."
go clean -cache
go build ./...

# Run tests
print_status "Running tests..."
go test ./...

# If we got here, everything worked
print_status "Rename completed successfully!"
print_status "The changes are on branch: $TEMP_BRANCH"
print_warning "To keep the changes:"
echo "1. Review the changes: git diff main"
echo "2. If satisfied: git checkout main && git merge $TEMP_BRANCH && git branch -d $TEMP_BRANCH"
print_warning "To discard the changes:"
echo "git checkout main && git branch -D $TEMP_BRANCH" 
