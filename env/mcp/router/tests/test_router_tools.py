import unittest
import sys
from pathlib import Path
from unittest.mock import Mock, MagicMock

# Add the router directory to sys.path to allow absolute imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from core.registry import MCPRegistry, Tool
from core.downstream_manager import DownstreamManager
from tools.router_tools import make_tools

class TestRouterTools(unittest.TestCase):

    def setUp(self):
        self.registry = MCPRegistry()
        self.downstream_manager = Mock(spec=DownstreamManager)
        self.router_tools = make_tools(self.registry, self.downstream_manager)
        for tname, tool in self.router_tools.items():
            self.registry.register_tool(tname, tool)

    def test_connect_local_server(self):
        tool_name = "ROUTER_connect_local_server"
        args = {"name": "test_local", "cmd": ["python", "server.py", "--stdio"]}
        
        result = self.registry.get_tool(tool_name).run(args)
        self.assertEqual(result, {"ok": True, "connected": "test_local"})
        self.downstream_manager.connect_local.assert_called_once_with("test_local", ["python", "server.py", "--stdio"])

    def test_connect_remote_server(self):
        tool_name = "ROUTER_connect_remote_server"
        args = {"name": "test_remote", "image": "myorg/remote_mcp:latest", "extra_args": ["--env", "X=1"]}
        
        self.downstream_manager.connect_remote_docker.return_value = ("container_123", Mock())

        result = self.registry.get_tool(tool_name).run(args)
        self.assertEqual(result, {"ok": True, "container_id": "container_123", "connected": "test_remote"})
        self.downstream_manager.connect_remote_docker.assert_called_once_with("test_remote", "myorg/remote_mcp:latest", extra_args=["--env", "X=1"])

    def test_list_registry_tool(self):
        tool_name = "ROUTER_list_registry"
        self.registry.snapshot = Mock(return_value={"tools": [], "resources": [], "agents": []})
        
        result = self.registry.get_tool(tool_name).run({})
        self.assertEqual(result, {"tools": [], "resources": [], "agents": []})
        self.registry.snapshot.assert_called_once()

    def test_disconnect_local_server(self):
        tool_name = "ROUTER_disconnect_server"
        args = {"name": "test_local", "type": "local"}
        self.downstream_manager.disconnect_local.return_value = True

        result = self.registry.get_tool(tool_name).run(args)
        self.assertEqual(result, {"ok": True})
        self.downstream_manager.disconnect_local.assert_called_once_with("test_local")

    def test_disconnect_remote_server(self):
        tool_name = "ROUTER_disconnect_server"
        args = {"name": "test_remote", "type": "remote"}
        self.downstream_manager.disconnect_remote.return_value = True

        result = self.registry.get_tool(tool_name).run(args)
        self.assertEqual(result, {"ok": True})
        self.downstream_manager.disconnect_remote.assert_called_once_with("test_remote")

if __name__ == '__main__':
    unittest.main()