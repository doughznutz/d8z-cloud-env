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
│   └── dynamic_loader.py         # dynamically loads tools, resources, and agents
├── tools/                        # dynamically loaded tool modules
├── resources/                    # dynamically loaded resource modules
├── agents/                       # dynamically loaded agent modules
└── README.md
```

## How to Run

1.  Start the router:
    ```bash
    python router.py --port 3456
    ```

2.  From another process, you can connect to the router's MCP server and call its management tools.

## Extending the Router with Dynamic Components

The MCP Router is designed to be highly extensible through dynamic loading of components. You can add new tools, resources, and agents by simply creating Python files in their respective directories (`tools/`, `resources/`, `agents/`). The router will automatically discover, load, and hot-reload these components without requiring a restart.

### Adding New Tools

To add a new tool:
1.  Create a new Python file (e.g., `my_new_tools.py`) in the `env/mcp/router/tools/` directory.
2.  Define a function named `make_tools` that accepts `registry` and `downstream_manager` as arguments.
3.  Inside `make_tools`, create instances of `Tool` (imported from `core.registry`) and return them in a dictionary.

    ```python
    from core.registry import MCPRegistry, Tool
    from core.downstream_manager import DownstreamManager

    def make_tools(registry: MCPRegistry, downstream: DownstreamManager):
        tools = {}

        def my_new_tool_run_fn(args):
            # Your tool logic here
            return {"status": "success", "message": "Hello from my new tool!", "args": args}

        tools["MY_PREFIX_my_new_tool"] = Tool(
            name="MY_PREFIX_my_new_tool",
            description="A description of what my new tool does.",
            parameters=[{"name": "param1", "type": "str", "required": True}],
            run_fn=my_new_tool_run_fn
        )
        return tools
    ```

### Adding New Resources

To add a new resource:
1.  Create a new Python file (e.g., `my_new_resources.py`) in the `env/mcp/router/resources/` directory.
2.  Define a function named `make_resources` that accepts `registry` and `downstream_manager` as arguments.
3.  Inside `make_resources`, create instances of `Resource` (imported from `core.registry`) and return them in a dictionary.

    ```python
    from core.registry import MCPRegistry, Resource
    from core.downstream_manager import DownstreamManager

    def make_resources(registry: MCPRegistry, downstream: DownstreamManager):
        resources = {}

        def my_new_resource_access_fn(args):
            # Your resource access logic here
            return {"state": "resource_is_fine", "args": args}

        resources["MY_PREFIX_my_new_resource"] = Resource(
            name="MY_PREFIX_my_new_resource",
            description="A description of what my new resource provides.",
            access_fn=my_new_resource_access_fn
        )
        return resources
    ```

### Adding New Agents

To add a new agent:
1.  Create a new Python file (e.g., `my_new_agents.py`) in the `env/mcp/router/agents/` directory.
2.  Define a function named `make_agents` that accepts `registry` and `downstream_manager` as arguments.
3.  Inside `make_agents`, create instances of `Agent` (imported from `core.registry`) and return them in a dictionary.

    ```python
    from core.registry import MCPRegistry, Agent
    from core.downstream_manager import DownstreamManager

    def make_agents(registry: MCPRegistry, downstream: DownstreamManager):
        agents = {}

        def my_new_agent_run_fn(args):
            # Your agent run logic here
            return {"action": "agent_completed_task", "args": args}

        agents["MY_PREFIX_my_new_agent"] = Agent(
            name="MY_PREFIX_my_new_agent",
            description="A description of what my new agent does.",
            run_fn=my_new_agent_run_fn
        )
        return agents
    ```

### Naming Conventions

It is recommended to prefix the names of your tools, resources, and agents with a unique identifier (e.g., `ENV_`, `PROJECT_`, `ROUTER_`) to avoid naming conflicts and clearly indicate their origin. For example, `ENV_get_environment`.

### Hot-Reloading

Changes to Python files in the `tools/`, `resources/`, and `agents/` directories will be automatically detected and reloaded by the router every 2 seconds. This allows for rapid development and iteration without needing to restart the main router process. When a file is modified, its old components are unregistered, and the new ones are registered.
