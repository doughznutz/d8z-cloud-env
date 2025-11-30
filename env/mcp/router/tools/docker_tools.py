import logging
import os
import subprocess

from core.registry import MCPRegistry, Tool
from core.downstream_manager import DownstreamManager # This might not be needed for simple tools

log = logging.getLogger(__name__)

def make_tools(registry: MCPRegistry, downstream: DownstreamManager):
    tools = {}

    def check_docker_socket(args):
        """Checks if the Docker socket is available and globally read/write."""
        socket_path = "/var/run/docker.sock"
        if not os.path.exists(socket_path):
            return {"status": "error", "message": "Docker socket not found."}

        mode = os.stat(socket_path).st_mode
        # Check for read (4) and write (2) permissions for 'others'
        if not (mode & 0o004 and mode & 0o002):
            return {
                "status": "error",
                "message": "Docker socket is not globally readable and writable. To fix this, run: sudo chmod o+rw /var/run/docker.sock"
            }
            
        return {"status": "success", "message": "Docker socket is accessible and has global read/write permissions."}

    tools["DOCKER_check_docker_socket"] = Tool(
        name="DOCKER_check_docker_socket",
        description=check_docker_socket.__doc__.strip(),
        parameters=[],
        run_fn=check_docker_socket
    )

    def launch_ide(args):
        """Launches the specified IDE service using docker compose."""
        ide_name = args.get("ide_name")
        if ide_name not in ["vnc", "vscode", "codeserver"]:
            return {"status": "error", "message": "Invalid IDE specified."}
        subprocess.run(["docker", "compose", "up", "-d", ide_name], check=True)
        return {"status": "success", "message": f"IDE '{ide_name}' is starting."}

    tools["DOCKER_launch_ide"] = Tool(
        name="DOCKER_launch_ide",
        description=launch_ide.__doc__.strip(),
        parameters=[{"name": "ide_name", "type": "str", "required": True}],
        run_fn=launch_ide
    )

    return tools
