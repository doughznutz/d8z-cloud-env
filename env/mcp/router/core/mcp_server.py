# core/mcp_server.py
# Simple TCP MCP server that exposes the router registry via JSON per-line protocol.
import socket
import threading
import json
from typing import Dict, Any
from .registry import MCPRegistry

class MCPServer:
    def __init__(self, registry: MCPRegistry, host: str = "0.0.0.0", port: int = 3456):
        self.registry = registry
        self.host = host
        self.port = port
        self._sock = None
        self._running = False

    def start(self):
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind((self.host, self.port))
        s.listen(5)
        self._sock = s
        self._running = True
        threading.Thread(target=self._accept_loop, daemon=True).start()
        print(f"MCP Router server listening on {self.host}:{self.port}")

    def _accept_loop(self):
        while self._running:
            try:
                conn, addr = self._sock.accept()
                threading.Thread(target=self._handle_conn, args=(conn, addr), daemon=True).start()
            except Exception as e:
                print("accept loop error", e)

    def _handle_conn(self, conn: socket.socket, addr):
        f = conn.makefile("rwb")
        while True:
            line = f.readline()
            if not line:
                break
            try:
                req = json.loads(line.decode("utf-8"))
                resp = self._handle_request(req)
            except Exception as e:
                resp = {"error": str(e)}
            out = (json.dumps(resp) + "\n").encode("utf-8")
            f.write(out)
            f.flush()
        try:
            conn.close()
        except Exception:
            pass

    def _handle_request(self, req: Dict[str, Any]) -> Dict[str, Any]:
        t = req.get("type")
        if t == "list_all":
            return self.registry.snapshot()
        if t == "list_tools":
            return {"tools": self.registry.list_tools()}
        if t == "list_resources":
            return {"resources": self.registry.list_resources()}
        if t == "list_agents":
            return {"agents": self.registry.list_agents()}
        if t == "run_tool":
            name = req.get("name")
            args = req.get("args", {})
            tool = self.registry.get_tool(name)
            if not tool:
                return {"error": f"tool not found: {name}"}
            try:
                result = tool.run(args)
                return {"ok": True, "result": result}
            except Exception as e:
                return {"error": f"tool run error: {e}"}
        if t == "access_resource":
            name = req.get("name")
            args = req.get("args", {})
            r = self.registry.get_resource(name)
            if not r:
                return {"error": f"resource not found: {name}"}
            try:
                result = r.access(args)
                return {"ok": True, "result": result}
            except Exception as e:
                return {"error": f"resource access error: {e}"}
        if t == "run_agent":
            name = req.get("name")
            args = req.get("args", {})
            a = self.registry.get_agent(name)
            if not a:
                return {"error": f"agent not found: {name}"}
            try:
                result = a.run(args)
                return {"ok": True, "result": result}
            except Exception as e:
                return {"error": f"agent run error: {e}"}
        return {"error": f"unknown request type: {t}"}
