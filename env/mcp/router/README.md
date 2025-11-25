# MCP Router

This directory contains a Python-based MCP (Multi-Control Plane) router.

## Project Layout

```
mcp_router/
├── router.py                     # main entrypoint
├── core/
│   ├── registry.py               # central MCP registry (tools/resources/agents)
│   ├── stdio_client.py           # connect to local server via stdio subprocess
│   ├── tcp_client.py             # connect to remote server via TCP
│   ├── downstream_manager.py     # manages downstream servers & prefixes
│   ├── mcp_server.py             # router's own MCP TCP server (simple JSON)
│   └── config_tools.py           # router-management tools implementations
└── README.md
```

## How to Run

1.  Start the router:
    ```bash
    python router.py --port 3456
    ```

2.  From another process, you can connect to the router's MCP server and call its management tools.
