# tools/router_tools.py
# High-level router management tools that will be registered into the registry as ROUTER_* tools.
import logging
from core.registry import MCPRegistry, Tool
from core.downstream_manager import DownstreamManager

log = logging.getLogger(__name__)

def make_tools(registry: MCPRegistry, downstream: DownstreamManager):
    tools = {}

    def connect_local_tool(args):
        # args: {"name": "analytics", "cmd": ["python","analytics_mcp.py","--stdio"]}
        name = args["name"]
        cmd = args["cmd"]
        downstream.connect_local(name, cmd)
        return {"ok": True, "connected": name}

    tools["ROUTER_connect_local_server"] = Tool(
        name="ROUTER_connect_local_server",
        description="Start a local MCP server subprocess and connect via stdio. args: name, cmd",
        run_fn=connect_local_tool
    )

    def connect_remote_tool(args):
        # args: {"name":"analytics","image":"myorg/analytics_mcp:latest","extra_args": ["--env", "X=1"]}
        name = args["name"]
        image = args["image"]
        extra = args.get("extra_args")
        cid, client = downstream.connect_remote_docker(name, image, extra_args=extra)
        return {"ok": True, "container_id": cid, "connected": name}

    tools["ROUTER_connect_remote_server"] = Tool(
        name="ROUTER_connect_remote_server",
        description="Launch docker container for remote server and connect to it. args: name, image, extra_args",
        run_fn=connect_remote_tool
    )

    def list_registry_tool(args):
        return registry.snapshot()

    tools["ROUTER_list_registry"] = Tool(
        name="ROUTER_list_registry",
        description="Return a snapshot of registry",
        run_fn=list_registry_tool
    )

    def disconnect_tool(args):
        # args: {"name":"analytics","type":"local"|"remote"}
        name = args["name"]
        typ = args.get("type","local")
        ok=False
        if typ=="local":
            ok = downstream.disconnect_local(name)
        else:
            ok = downstream.disconnect_remote(name)
        return {"ok": ok}
    tools["ROUTER_disconnect_server"] = Tool(
        name="ROUTER_disconnect_server",
        description="Disconnect a named downstream server",
        run_fn=disconnect_tool
    )

    def connect_service_tool(args):
        """Connects the router to a running Docker Compose service by its name."""
        service_name = args.get("service_name")
        if not service_name:
            return {"status": "error", "message": "Missing required argument: service_name"}

        try:
            log.info(f"Attempting to connect to MCP interface for service '{service_name}' at {service_name}:3456...")
            downstream.connect_service(name=service_name, host=service_name, port=3456)
            log.info(f"Successfully connected to service '{service_name}'.")
            return {"status": "success", "message": f"Successfully connected to service '{service_name}'."}
        except Exception as e:
            log.error(f"Error connecting to service '{service_name}': {e}", exc_info=True)
            return {"status": "error", "message": f"Failed to connect to service '{service_name}'.", "error": str(e)}

    tools["ROUTER_connect_service"] = Tool(
        name="ROUTER_connect_service",
        description=connect_service_tool.__doc__.strip(),
        parameters=[{"name": "service_name", "type": "str", "required": True}],
        run_fn=connect_service_tool
    )

    return tools
