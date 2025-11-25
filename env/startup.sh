#!/bin/bash
set -e

# Start the MCP Router in the background, passing arguments securely
echo "Starting MCP Router..."
/opt/venv_mcp/bin/python3 /app/mcp/router/router.py "$@" &

# Wait a moment for the router to initialize
sleep 2

# Start the Node.js client in the foreground, passing arguments securely
echo "Starting Node.js client..."
node /app/entrypoint/index.js "$@"
