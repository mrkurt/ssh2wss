#!/bin/bash

# Exit on error
set -e

# Run semgrep with critical rules
echo "Running semgrep on application code..."
semgrep --config rules/semgrep core/ cmd/

# Exit with success only if semgrep succeeds
exit 0 