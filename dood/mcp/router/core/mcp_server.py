# core/mcp_server.py
# Simple TCP MCP server that exposes the router registry via JSON per-line protocol.
import logging
import socket
import threading
import json
from pathlib import Path
from typing import Dict, Any
from .registry import MCPRegistry
from .downstream_manager import DownstreamManager
from .dynamic_loader import DynamicLoader

log = logging.getLogger(__name__)

class MCPServer:
    def __init__(self, host: str = "0.0.0.0", port: int = 3456):
        self.registry = MCPRegistry()
        self.downstream_manager = DownstreamManager(self.registry)
        
        # The tools directory is assumed to be ../tools relative to this file's directory
        tools_dir = Path(__file__).parent.parent / "tools"
        resources_dir = Path(__file__).parent.parent / "resources"
        agents_dir = Path(__file__).parent.parent / "agents"
        self.dynamic_loader = DynamicLoader(self.registry, self.downstream_manager, tools_dir, resources_dir, agents_dir)
        
        self.host = host
        self.port = port
        self._sock = None
        self._running = False

    def start(self):
        # Load tools before starting the server
        log.info("Loading dynamic components...")
        self.dynamic_loader.load_components()
        log.info(f"Loaded tools: {self.registry.list_tools()}")
        log.info(f"Loaded resources: {self.registry.list_resources()}")
        log.info(f"Loaded agents: {self.registry.list_agents()}")

        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind((self.host, self.port))
        s.listen(5)
        self._sock = s
        self._running = True
        threading.Thread(target=self._accept_loop, daemon=True).start()
        log.info(f"MCP Router server listening on {self.host}:{self.port}")
        # Start watching for changes
        self.dynamic_loader.watch_for_changes()

    def _accept_loop(self):
        while self._running:
            try:
                conn, addr = self._sock.accept()
                threading.Thread(target=self._handle_conn, args=(conn, addr), daemon=True).start()
            except Exception as e:
                log.error("accept loop error", exc_info=e)

    def _handle_conn(self, conn: socket.socket, addr):
        log.info(f"Connection from {addr}")
        f = conn.makefile("rwb")
        while True:
            line = f.readline()
            if not line:
                break
            try:
                req = json.loads(line.decode("utf-8"))
                log.debug(f"Request from {addr}: {req}")
                resp = self._handle_request(req)
            except Exception as e:
                resp = {"error": str(e)}
                log.error(f"Error handling request: {e}", exc_info=True)
            out = (json.dumps(resp) + "\n").encode("utf-8")
            f.write(out)
            f.flush()
        try:
            conn.close()
        except Exception:
            pass
        log.info(f"Connection from {addr} closed")

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
                log.error(f"tool run error: {e}", exc_info=True)
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
                log.error(f"resource access error: {e}", exc_info=True)
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
                log.error(f"agent run error: {e}", exc_info=True)
                return {"error": f"agent run error: {e}"}
        return {"error": f"unknown request type: {t}"}
