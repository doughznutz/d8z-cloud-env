# MCP Server URL
MCP_URL="http://cloud:8080/mcp"
echo "Step 1: Initializing MCP server..."
INIT_RESPONSE=$(curl -s -X POST $MCP_URL \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "capabilities": {
        "tools": {},
        "resources": {},
        "prompts": {}
      },
      "clientInfo": {
        "name": "test-client",
        "version": "1.0.0"
      }
    }
  }' | jq '.')
echo "Initialization response: $INIT_RESPONSE"
# Extract session ID
SESSION_ID=$(curl -s -X POST $MCP_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{"tools":{},"resources":{},"prompts":{}},"clientInfo":{"name":"test-client","version":"1.0.0"}}}' \
  -v 2>&1 | grep -i "mcp-session-id" | cut -d' ' -f3)
echo "Session ID: $SESSION_ID"
echo -e "\nStep 2: Listing tools..."
curl -s $MCP_URL \
  -H "Content-Type: application/json" \
  -H "Accept: application/json,text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | jq '.'
echo -e "\nStep 3: Analyzing cluster problems..."
curl -s -X POST $MCP_URL \
  -H "Content-Type: application/json" \
  -H "Accept: application/json,text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"analyze","arguments":{"explain":false}}}' | jq '.'
echo -e "\nStep 4: Analyzing Pods only..."
curl -s -X POST $MCP_URL \
  -H "Content-Type: application/json" \
  -H "Accept: application/json,text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"analyze","arguments":{"filters":["Pod"],"explain":false}}}' | jq '.'
echo -e "\nStep 5: Getting AI analysis with explanations..."
curl -s -X POST $MCP_URL \
  -H "Content-Type: application/json" \
  -H "Accept: application/json,text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"analyze","arguments":{"filters":["Pod"],"explain":true}}}' | jq '.'
echo -e "\nTesting completed!"