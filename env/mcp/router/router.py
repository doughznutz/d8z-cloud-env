# router.py
import argparse
import time
from core.registry import MCPRegistry
from core.downstream_manager import DownstreamManager
from core.mcp_server import MCPServer
from core.config_tools import make_router_tools

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=3456)
    parser.add_argument("--debug", action="store_true", help="Enable debug mode for verbose logging.")
    args = parser.parse_args()

    if args.debug:
        print("DEBUG mode enabled for MCP Router.")

    registry = MCPRegistry()
    downstream = DownstreamManager(registry=registry)

    # Register router management tools into registry
    router_tools = make_router_tools(registry, downstream)
    for tname, tool in router_tools.items():
        registry.register_tool(tname, tool)

    # Start router's MCP server endpoint
    server = MCPServer(registry=registry, host=args.host, port=args.port)
    server.start()

    print("Router running. Management tools:")
    for n in registry.list_tools():
        print(" -", n)

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("shutting down.")

if __name__ == "__main__":
    main()
