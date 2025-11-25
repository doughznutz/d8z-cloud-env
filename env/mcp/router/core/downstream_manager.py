# core/downstream_manager.py
import socket
import subprocess
import time
import json
from typing import Dict, Any, Optional
from .stdio_client import StdioMCPClient
from .tcp_client import TCPMCPClient
from .registry import MCPRegistry

def _find_free_port():
    s = socket.socket()
    s.bind(('', 0))
    port = s.getsockname()[1]
    s.close()
    return port

class DownstreamManager:
    def __init__(self, registry: MCPRegistry):
        self.registry = registry
        # keyed by name
        self._local_clients: Dict[str, StdioMCPClient] = {}
        self._tcp_clients: Dict[str, TCPMCPClient] = {}
        self._docker_containers: Dict[str, str] = {}  # name -> container id

    def connect_local(self, name: str, cmd: list):
        prefix = f"{name.upper()}_"
        client = StdioMCPClient(name=name, cmd=cmd, registry=self.registry, prefix=prefix)
        client.start()
        self._local_clients[name] = client
        return client

    def disconnect_local(self, name: str):
        client = self._local_clients.pop(name, None)
        if client:
            client.stop()
            # remove registered objects with prefix
            pref = f"{name.upper()}_"
            for t in list(self.registry.list_tools()):
                if t.startswith(pref):
                    self.registry.remove_tool(t)
            for r in list(self.registry.list_resources()):
                if r.startswith(pref):
                    self.registry.remove_resource(r)
            for a in list(self.registry.list_agents()):
                if a.startswith(pref):
                    self.registry.remove_agent(a)
            return True
        return False

    def connect_remote_docker(self, name: str, image: str, extra_args: Optional[list] = None):
        """
        Launch docker container mapping a free host port (hostport) to container:3456,
        then connect via TCP to hostport.
        """
        host_port = _find_free_port()
        # build docker run command
        args = ["docker", "run", "-d", "-p", f"{host_port}:3456"]
        if extra_args:
            args += extra_args
        args += [image]
        proc = subprocess.run(args, capture_output=True, text=True)
        if proc.returncode != 0:
            raise RuntimeError(f"docker run failed: {proc.stderr}")
        container_id = proc.stdout.strip()
        # wait briefly for container to start
        time.sleep(1.0)
        prefix = f"{name.upper()}_"
        client = TCPMCPClient(name=name, host="127.0.0.1", port=host_port, registry=self.registry, prefix=prefix)
        client.connect()
        self._tcp_clients[name] = client
        self._docker_containers[name] = container_id
        return container_id, client

    def disconnect_remote(self, name: str):
        client = self._tcp_clients.pop(name, None)
        cid = self._docker_containers.pop(name, None)
        if client:
            client.close()
        # remove prefixed registry entries
        pref = f"{name.upper()}_"
        for t in list(self.registry.list_tools()):
            if t.startswith(pref):
                self.registry.remove_tool(t)
        for r in list(self.registry.list_resources()):
            if r.startswith(pref):
                self.registry.remove_resource(r)
        for a in list(self.registry.list_agents()):
            if a.startswith(pref):
                self.registry.remove_agent(a)
        if cid:
            subprocess.run(["docker", "rm", "-f", cid])
            return True
        return False

    def list_downstreams(self):
        return {
            "local": list(self._local_clients.keys()),
            "remote": list(self._tcp_clients.keys())
        }
