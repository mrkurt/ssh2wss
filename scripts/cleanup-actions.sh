#!/bin/bash

# Ensure gh CLI is available
if ! command -v gh &> /dev/null; then
    echo "GitHub CLI (gh) is not installed. Please install it first."
    exit 1
fi

# Ensure we're authenticated
if ! gh auth status &> /dev/null | cat; then
    echo "Not authenticated with GitHub. Please run 'gh auth login' first."
    exit 1
fi

echo "Fetching workflow runs..."
# List all workflow runs, filter non-main branches, and delete them
gh run list --limit 500 --json databaseId,headBranch,status,conclusion | cat | \
    jq -r '.[] | select(.headBranch != "main") | .databaseId' | \
    while read -r run_id; do
        echo "Deleting run $run_id..."
        gh api -X DELETE "/repos/{owner}/{repo}/actions/runs/$run_id" | cat
    done

echo "Done cleaning up workflow runs." 