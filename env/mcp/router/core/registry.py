# core/registry.py
from typing import Dict, Any, Callable
import threading

class Tool:
    def __init__(self, name: str, description: str, run_fn: Callable, parameters: list = []):
        self.name = name
        self.description = description
        self.run = run_fn
        self.parameters = parameters

class Resource:
    def __init__(self, name: str, description: str, access_fn: Callable):
        self.name = name
        self.description = description
        self.access = access_fn

class Agent:
    def __init__(self, name: str, description: str, run_fn: Callable):
        self.name = name
        self.description = description
        self.run = run_fn

class MCPRegistry:
    def __init__(self):
        self._tools: Dict[str, Tool] = {}
        self._resources: Dict[str, Resource] = {}
        self._agents: Dict[str, Agent] = {}
        self._lock = threading.RLock()

    # Tools
    def register_tool(self, name: str, tool: Tool):
        with self._lock:
            self._tools[name] = tool

    def remove_tool(self, name: str):
        with self._lock:
            if name in self._tools:
                del self._tools[name]

    def list_tools(self):
        with self._lock:
            return list(self._tools.keys())

    def get_tool(self, name: str):
        with self._lock:
            return self._tools.get(name)

    # Resources
    def register_resource(self, name: str, resource: Resource):
        with self._lock:
            self._resources[name] = resource

    def remove_resource(self, name: str):
        with self._lock:
            if name in self._resources:
                del self._resources[name]

    def list_resources(self):
        with self._lock:
            return list(self._resources.keys())

    def get_resource(self, name: str):
        with self._lock:
            return self._resources.get(name)

    # Agents
    def register_agent(self, name: str, agent: Agent):
        with self._lock:
            self._agents[name] = agent

    def remove_agent(self, name: str):
        with self._lock:
            if name in self._agents:
                del self._agents[name]

    def list_agents(self):
        with self._lock:
            return list(self._agents.keys())

    def get_agent(self, name: str):
        with self._lock:
            return self._agents.get(name)

    # Full snapshot (for listing to clients)
    def snapshot(self):
        with self._lock:
            return {
                "tools": [{"name": k, "description": v.description, "parameters": v.parameters} for k, v in self._tools.items()],
                "resources": [{"name": k, "description": v.description} for k, v in self._resources.items()],
                "agents": [{"name": k, "description": v.description} for k, v in self._agents.items()]
            }
