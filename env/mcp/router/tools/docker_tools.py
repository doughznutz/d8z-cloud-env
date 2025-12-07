import logging
import os
import subprocess
import json
import time

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
        """Launches the specified IDE service using make."""
        ide_name = args.get("ide_name")
        valid_ides = ["vnc", "vscode", "codeserver"]
        if ide_name not in valid_ides:
            return {"status": "error", "message": f"Invalid IDE specified. Must be one of {valid_ides}"}

        command = ["make", f"up-{ide_name}"]
        try:
            result = subprocess.run(command, check=True, capture_output=True, text=True)
            log.info(f"Successfully ran command: '{' '.join(command)}'. Output:\n{result.stdout}")
            return {"status": "success", "message": f"IDE '{ide_name}' is starting via '{' '.join(command)}'."}
        except subprocess.CalledProcessError as e:
            log.error(f"Error running command: '{' '.join(command)}'. Error:\n{e.stderr}")
            return {"status": "error", "message": f"Failed to start IDE '{ide_name}'.", "error": e.stderr}
        except FileNotFoundError:
            log.error("The 'make' command was not found.")
            return {"status": "error", "message": "The 'make' command is not installed or not in the system's PATH."}

    tools["DOCKER_launch_ide"] = Tool(
        name="DOCKER_launch_ide",
        description=launch_ide.__doc__.strip(),
        parameters=[{"name": "ide_name", "type": "str", "required": True}],
        run_fn=launch_ide
    )

    def launch_service(args):
        """Launches a specified deployment service and verifies it is running."""
        service_name = args.get("service_name")
        valid_services = ["cloud", "docker", "github"]
        if service_name not in valid_services:
            return {"status": "error", "message": f"Invalid service specified. Must be one of {valid_services}"}

        # 1. Launch service via make
        make_command = ["make", f"up-{service_name}"]
        try:
            # Using a long timeout because this can involve downloading images
            result = subprocess.run(make_command, check=True, capture_output=True, text=True, timeout=300)
            log.info(f"Successfully ran command: '{' '.join(make_command)}'. Output:\n{result.stdout}")
        except subprocess.CalledProcessError as e:
            log.error(f"Error running command: '{' '.join(make_command)}'. Error:\n{e.stderr}")
            return {"status": "error", "message": f"Failed to start service '{service_name}'.", "error": e.stderr}
        except subprocess.TimeoutExpired as e:
            log.error(f"Timeout running command: '{' '.join(make_command)}'.")
            return {"status": "error", "message": f"Timeout running 'make up-{service_name}'. It may be taking a long time to download or build.", "error": str(e)}
        except FileNotFoundError:
            log.error("The 'make' command was not found.")
            return {"status": "error", "message": "The 'make' command is not installed or not in the system's PATH."}

        # 2. Verify service is running
        time.sleep(3) # Give the service a moment to stabilize or crash
        
    def launch_service(args):
        """Launches a service and polls its status to ensure it stays running."""
        service_name = args.get("service_name")
        valid_services = ["cloud", "docker", "github"]
        if service_name not in valid_services:
            return {"status": "error", "message": f"Invalid service specified. Must be one of {valid_services}"}

        # 1. Launch service detached, ensuring it's built
        up_command = ["docker", "compose", "up", "-d", "--build", service_name]
        try:
            subprocess.run(up_command, check=True, capture_output=True, text=True, timeout=300)
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as e:
            # If the 'up' command itself fails, get the logs and return
            logs_command = ["docker", "compose", "logs", "--no-color", "--tail=50", service_name]
            logs_result = subprocess.run(logs_command, capture_output=True, text=True)
            error_logs = logs_result.stdout or logs_result.stderr
            return {
                "status": "error",
                "message": f"Command 'docker compose up' failed for service '{service_name}'.",
                "error": f"Details: {getattr(e, 'stderr', e)}\n\nContainer logs:\n{error_logs}"
            }

        # 2. Poll for stability
        polling_duration_seconds = 15
        polling_interval_seconds = 3
        start_time = time.time()
        
        while time.time() - start_time < polling_duration_seconds:
            time.sleep(polling_interval_seconds)
            try:
                ps_command = ["docker", "compose", "ps", "--format", "json", service_name]
                ps_result = subprocess.run(ps_command, check=True, capture_output=True, text=True)
                ps_output = ps_result.stdout.strip()
                
                if not ps_output:
                    continue # Not up yet, keep polling

                service_info = json.loads(ps_output.split('\n')[0])
                service_state = service_info.get("State", "").lower()

                if "running" in service_state:
                    log.info(f"Service '{service_name}' is running.")
                    return {"status": "success", "message": f"Service '{service_name}' launched and is running."}
                
                if "exited" in service_state or "dead" in service_state:
                    log.error(f"Service '{service_name}' entered failed state: {service_state}")
                    break # Break and go to failure reporting

            except (subprocess.CalledProcessError, json.JSONDecodeError, IndexError) as e:
                 log.warning(f"Polling error for service '{service_name}', will retry: {e}")
                 # Continue polling
        
        # 3. If loop finishes or is broken, it's a failure.
        logs_command = ["docker", "compose", "logs", "--no-color", "--tail=50", service_name]
        logs_result = subprocess.run(logs_command, capture_output=True, text=True)
        error_logs = logs_result.stdout or logs_result.stderr
        return {
            "status": "error",
            "message": f"Service '{service_name}' failed to become stable within {polling_duration_seconds} seconds.",
            "error": f"Container logs:\n{error_logs}"
        }

    tools["DOCKER_launch_service"] = Tool(
        name="DOCKER_launch_service",
        description=launch_service.__doc__.strip(),
        parameters=[{"name": "service_name", "type": "str", "required": True}],
        run_fn=launch_service
    )

    tools["DOCKER_launch_service"] = Tool(
        name="DOCKER_launch_service",
        description=launch_service.__doc__.strip(),
        parameters=[{"name": "service_name", "type": "str", "required": True}],
        run_fn=launch_service
    )

    return tools
