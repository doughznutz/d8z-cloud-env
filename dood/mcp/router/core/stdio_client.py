# core/stdio_client.py
# Connect to a local python MCP server launched as a subprocess using stdio JSON messages.
import logging
import subprocess
import threading
import json
import time
from typing import Dict, Any, Optional, Callable
from .registry import MCPRegistry

log = logging.getLogger(__name__)

class StdioMCPClient:
    def __init__(self, name: str, cmd: list, registry: MCPRegistry, prefix: str):
        """
        cmd: list for subprocess (e.g. ["python", "my_mcp_server.py", "--stdio"])
        prefix: e.g. 'ANALYTICS_'
        """
        self.name = name
        self.cmd = cmd
        self.proc: Optional[subprocess.Popen] = None
        self._reader_thread: Optional[threading.Thread] = None
        self.registry = registry
        self.prefix = prefix
        self._alive = False

    def start(self):
        log.info(f"Starting stdio client '{self.name}' with command: {' '.join(self.cmd)}")
        self.proc = subprocess.Popen(
            self.cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            bufsize=0
        )
        self._alive = True
        self._reader_thread = threading.Thread(target=self._read_loop, daemon=True)
        self._reader_thread.start()
        # Give the process a moment, then init handshake
        time.sleep(0.2)
        self._do_init()

    def _do_init(self):
        # ask the server to list tools/resources/agents
        res = self.call({"type": "list_all"})
        if not res:
            log.warning(f"Failed to get capabilities from stdio client '{self.name}'")
            return
        
        log.info(f"Registering capabilities from '{self.name}'")
        # expects res like: {"tools": [{"name":"summarize"}], "resources":[...], "agents":[...]}
        for t in res.get("tools", []):
            name = f"{self.prefix}{t['name']}"
            # create a proxy tool that calls this server when invoked
            def make_run(n):
                def run(args):
                    return self.call({"type": "run_tool", "name": n, "args": args})
                return run
            tool_obj = None
            # lazy Tool object to store proxy run
            from .registry import Tool
            tool_obj = Tool(
                name=name, 
                description=t.get("description", ""), 
                parameters=t.get("parameters", []), 
                run_fn=make_run(t["name"])
            )
            self.registry.register_tool(name, tool_obj)

        for r in res.get("resources", []):
            name = f"{self.prefix}{r['name']}"
            from .registry import Resource
            def make_access(n):
                def access(args):
                    return self.call({"type": "access_resource", "name": n, "args": args})
                return access
            res_obj = Resource(name=name, description=r.get("description", ""), access_fn=make_access(r["name"]))
            self.registry.register_resource(name, res_obj)

        for a in res.get("agents", []):
            name = f"{self.prefix}{a['name']}"
            from .registry import Agent
            def make_run(n):
                def run(args):
                    return self.call({"type": "run_agent", "name": n, "args": args})
                return run
            agent_obj = Agent(name=name, description=a.get("description", ""), run_fn=make_run(a["name"]))
            self.registry.register_agent(name, agent_obj)

    def call(self, message: Dict[str, Any], timeout: float = 10.0) -> Optional[Dict[str, Any]]:
        if not self.proc or self.proc.stdin is None or self.proc.stdout is None:
            return None
        try:
            raw = json.dumps(message).encode("utf-8") + b"\n"
            self.proc.stdin.write(raw)
            self.proc.stdin.flush()

            # simple single-line response semantics
            line = self.proc.stdout.readline()
            if not line:
                return None
            return json.loads(line.decode("utf-8"))
        except Exception as e:
            log.error("stdio call error:", exc_info=e)
            return None

    def _read_loop(self):
        # optional: read stderr and print for debugging
        if not self.proc:
            return
        while self._alive:
            try:
                err = self.proc.stderr.readline()
                if not err:
                    time.sleep(0.1)
                    continue
                if err:
                    log.info(f"[{self.name} stderr] {err.decode('utf-8').rstrip()}")
            except Exception:
                break

    def stop(self):
        log.info(f"Stopping stdio client '{self.name}'")
        self._alive = False
        if self.proc:
            try:
                self.proc.terminate()
                self.proc.wait(timeout=2)
            except Exception:
                try:
                    self.proc.kill()
                except Exception:
                    pass
            self.proc = None
