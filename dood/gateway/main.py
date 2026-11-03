# gateway/main.py
# HTTP(S) MCP gateway: translates real MCP-over-HTTP JSON-RPC into calls
# against the internal TCP router (127.0.0.1:3456), so external MCP clients
# can reach the same tool registry the CLI and dashboard use. Published
# directly on a host port (bypasses Traefik on purpose, see docker-compose).
import json
import os
import sys

sys.path.insert(0, "/app/mcp")

from fastapi import FastAPI, Header, Request
from fastapi.responses import JSONResponse

from client import call_router

app = FastAPI(title="D8Z DOOD MCP Gateway", version="1.0.0")

GATEWAY_TOKEN = os.getenv("MCP_GATEWAY_TOKEN", "")


def _auth_error(authorization):
    if not GATEWAY_TOKEN:
        return JSONResponse(
            status_code=503,
            content={"detail": "Gateway not configured: set MCP_GATEWAY_TOKEN in .env"},
        )
    if authorization != f"Bearer {GATEWAY_TOKEN}":
        return JSONResponse(status_code=401, content={"detail": "Invalid or missing bearer token"})
    return None


def _rpc_error(request_id, message, code=-32000):
    if request_id is None:
        return JSONResponse(status_code=204, content=None)
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": str(message)}}


def _rpc_result(request_id, result):
    if request_id is None:
        return JSONResponse(status_code=204, content=None)
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def _tool_input_schema(tool):
    properties = {p["name"]: {"type": "string"} for p in tool.get("parameters", [])}
    required = [p["name"] for p in tool.get("parameters", []) if p.get("required")]
    return {"type": "object", "properties": properties, "required": required}


@app.get("/")
def health():
    return {"status": "D8Z DOOD MCP Gateway is running"}


@app.post("/mcp")
async def mcp_rpc(request: Request, authorization: str = Header(default=None)):
    auth_error = _auth_error(authorization)
    if auth_error:
        return auth_error

    body = await request.json()
    method = body.get("method")
    request_id = body.get("id")
    params = body.get("params") or {}

    try:
        if method == "initialize":
            result = {
                "protocolVersion": "2025-03-26",
                "capabilities": {
                    "tools": {"listChanged": True},
                    "resources": {"subscribe": False},
                },
                "serverInfo": {"name": "D8Z DOOD MCP Gateway", "version": "1.0.0"},
            }

        elif method == "tools/list":
            snapshot = call_router({"type": "list_all"})
            result = {"tools": [
                {
                    "name": t["name"],
                    "description": t.get("description", ""),
                    "inputSchema": _tool_input_schema(t),
                }
                for t in snapshot.get("tools", [])
            ]}

        elif method == "tools/call":
            name = params.get("name")
            arguments = params.get("arguments", {})
            router_resp = call_router({"type": "run_tool", "name": name, "args": arguments})
            if "error" in router_resp:
                return _rpc_error(request_id, router_resp["error"])
            result = {
                "content": [{"type": "text", "text": json.dumps(router_resp.get("result"))}],
                "isError": False,
            }

        elif method == "resources/list":
            snapshot = call_router({"type": "list_all"})
            result = {"resources": [
                {"uri": f"dood://{r['name']}", "name": r["name"], "description": r.get("description", "")}
                for r in snapshot.get("resources", [])
            ]}

        elif method == "resources/read":
            uri = params.get("uri", "")
            name = uri.split("://", 1)[-1]
            router_resp = call_router({"type": "access_resource", "name": name})
            if "error" in router_resp:
                return _rpc_error(request_id, router_resp["error"])
            result = {"contents": [{"uri": uri, "text": json.dumps(router_resp.get("result"))}]}

        elif method in ("notifications/initialized", "shutdown", "exit"):
            if request_id is None:
                return JSONResponse(status_code=204, content=None)
            result = None

        else:
            return _rpc_error(request_id, f"Method not found: {method}", code=-32601)

    except (ConnectionError, OSError) as e:
        return JSONResponse(status_code=502, content={"detail": f"MCP router unreachable: {e}"})

    return _rpc_result(request_id, result)
