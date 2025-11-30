# core/tcp_client.py
# Connect to a remote MCP server over TCP socket using simple JSON per-line frames.
import logging
import socket
import json
import threading
from typing import Dict, Any, Optional
from .registry import MCPRegistry

log = logging.getLogger(__name__)

class TCPMCPClient:
    def __init__(self, name: str, host: str, port: int, registry: MCPRegistry, prefix: str):
        self.name = name
        self.host = host
        self.port = port
        self.registry = registry
        self.prefix = prefix
        self.sock: Optional[socket.socket] = None
        self._lock = threading.RLock()

    def connect(self, timeout=5.0):
        log.info(f"Connecting to TCP client '{self.name}' at {self.host}:{self.port}")
        s = socket.create_connection((self.host, self.port), timeout=timeout)
        self.sock = s
        # do handshake
        resp = self.call({"type":"list_all"})
        if resp:
            log.info(f"Registering capabilities from '{self.name}'")
            for t in resp.get("tools", []):
                name = f"{self.prefix}{t['name']}"
                from .registry import Tool
                def make_run(n):
                    def run(args):
                        return self.call({"type":"run_tool", "name": n, "args": args})
                    return run
                tool_obj = Tool(
                    name=name, 
                    description=t.get("description",""), 
                    parameters=t.get("parameters", []), 
                    run_fn=make_run(t["name"])
                )
                self.registry.register_tool(name, tool_obj)
            for r in resp.get("resources", []):
                name = f"{self.prefix}{r['name']}"
                from .registry import Resource
                def make_access(n):
                    def access(args):
                        return self.call({"type":"access_resource", "name": n, "args": args})
                    return access
                res_obj = Resource(name=name, description=r.get("description",""), access_fn=make_access(r["name"]))
                self.registry.register_resource(name, res_obj)
            for a in resp.get("agents", []):
                name = f"{self.prefix}{a['name']}"
                from .registry import Agent
                def make_run(n):
                    def run(args):
                        return self.call({"type":"run_agent", "name": n, "args": args})
                    return run
                agent_obj = Agent(name=name, description=a.get("description",""), run_fn=make_run(a["name"]))
                self.registry.register_agent(name, agent_obj)
        else:
            log.warning(f"Failed to get capabilities from TCP client '{self.name}'")
        return True

    def call(self, message: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        if not self.sock:
            raise RuntimeError("not connected")
        with self._lock:
            try:
                data = json.dumps(message).encode("utf-8") + b"\n"
                self.sock.sendall(data)
                # read until newline
                buf = b""
                while True:
                    chunk = self.sock.recv(4096)
                    if not chunk:
                        break
                    buf += chunk
                    if b"\n" in buf:
                        line, rest = buf.split(b"\n", 1)
                        # we don't handle rest bytes currently
                        return json.loads(line.decode("utf-8"))
                return None
            except Exception as e:
                log.error("tcp call error:", exc_info=e)
                return None

    def close(self):
        log.info(f"Closing TCP client '{self.name}'")
        try:
            if self.sock:
                self.sock.close()
        except Exception:
            pass
        self.sock = None
