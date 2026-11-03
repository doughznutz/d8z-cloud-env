# core/downstream_manager.py
import logging
import socket
import subprocess
import time
import json
from typing import Dict, Any, Optional
from .stdio_client import StdioMCPClient
from .tcp_client import TCPMCPClient
from .registry import MCPRegistry

log = logging.getLogger(__name__)

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
        log.info(f"Connecting to local downstream '{name}'")
        prefix = f"{name.upper()}_"
        client = StdioMCPClient(name=name, cmd=cmd, registry=self.registry, prefix=prefix)
        client.start()
        self._local_clients[name] = client
        return client

    def disconnect_local(self, name: str):
        log.info(f"Disconnecting local downstream '{name}'")
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

    def connect_service(self, name: str, host: str, port: int):
        """Connects to a pre-existing service via TCP."""
        log.info(f"Connecting to existing service '{name}' at {host}:{port}")
        if name in self._tcp_clients:
            log.warning(f"Service '{name}' is already connected.")
            return self._tcp_clients[name]

        prefix = f"{name.upper()}_"
        client = TCPMCPClient(name=name, host=host, port=port, registry=self.registry, prefix=prefix)
        client.connect()
        self._tcp_clients[name] = client
        return client

    def connect_remote_docker(self, name: str, image: str, extra_args: Optional[list] = None):
        """
        Launch docker container mapping a free host port (hostport) to container:3456,
        then connect via TCP to hostport.
        """
        log.info(f"Connecting to remote downstream '{name}' with image '{image}'")
        host_port = _find_free_port()
        # build docker run command
        args = ["docker", "run", "-d", "-p", f"{host_port}:3456"]
        if extra_args:
            args += extra_args
        args += [image]
        log.info(f"Running command: {' '.join(args)}")
        proc = subprocess.run(args, capture_output=True, text=True)
        if proc.returncode != 0:
            log.error(f"Docker run failed for image '{image}': {proc.stderr}")
            raise RuntimeError(f"docker run failed: {proc.stderr}")
        container_id = proc.stdout.strip()
        log.info(f"Container '{container_id}' started for downstream '{name}'")
        # wait briefly for container to start
        time.sleep(1.0)
        prefix = f"{name.upper()}_"
        client = TCPMCPClient(name=name, host="127.0.0.1", port=host_port, registry=self.registry, prefix=prefix)
        client.connect()
        self._tcp_clients[name] = client
        self._docker_containers[name] = container_id
        return container_id, client

    def disconnect_remote(self, name: str):
        log.info(f"Disconnecting remote downstream '{name}'")
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
            log.info(f"Removing container '{cid}' for downstream '{name}'")
            subprocess.run(["docker", "rm", "-f", cid])
            return True
        return False

    def list_downstreams(self):
        return {
            "local": list(self._local_clients.keys()),
            "remote": list(self._tcp_clients.keys())
        }
