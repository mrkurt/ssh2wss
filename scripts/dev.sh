#!/bin/bash

# Generate random high port and token
PORT=$((RANDOM % (65535-49152) + 49152))
TOKEN=$(openssl rand -hex 16)

echo -e "\n=== Development Mode ==="
echo "WebSocket URL: ws://localhost:$PORT"
echo "Auth Token: $TOKEN"
echo "===================="

# Export token for both server and client
export WSS_AUTH_TOKEN=$TOKEN

# Start server in background
go run cmd/flyssh/main.go server -port $PORT &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Start client
go run cmd/flyssh/main.go client -url "ws://localhost:$PORT"

# Cleanup server on exit
kill $SERVER_PID 