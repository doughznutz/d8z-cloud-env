import inspect
import os
import sys
import json
import argparse
import re
import subprocess
from pathlib import Path

# --- Helpers ---

def get_projects_root(user=None):
    """Gets the project root directory, creating it if it doesn't exist."""
    if not user:
        # Default to a safe path if user is not specified
        user_home = os.getenv('USER_HOME', '/root')
    else:
        user_home = f"/home/{user}"
        
    projects_root = Path(user_home) / 'projects'
    projects_root.mkdir(parents=True, exist_ok=True)
    return projects_root

def get_environment(key: str = None):
    """
    Returns the runtime environment variables as a structured object.
    If a 'key' is provided, it returns the value for that key only.
    """
    if key:
        return {"value": os.getenv(key)}
    return dict(os.environ)

def get_projects():
    """
    Lists the directories in the user's project folder, based on the USER environment variable.
    """
    user = os.getenv('USER')
    if not user:
        raise ValueError("USER environment variable must be set to locate projects directory.")
        
    projects_root = get_projects_root(user)
    subdirs = [d.name for d in projects_root.iterdir() if d.is_dir()]
    return {"projects": subdirs}

def create_dot_env_file():
    """Creates an empty .env file if it does not already exist."""
    Path('.env').touch(exist_ok=True)
    return {"status": "success", "message": ".env file created or already exists."}

def set_environment_variable(key: str, value: str):
    """Sets or updates a variable in the .env file."""
    if not key or value is None:
        raise ValueError("Missing 'key' or 'value' for update action.")
    
    env_file = Path('.env')
    if not env_file.exists():
        env_file.touch()
        
    content = env_file.read_text()
    if re.search(f"^{re.escape(key)}=", content, flags=re.MULTILINE):
        content = re.sub(f"^{re.escape(key)}=.*$", f"{key}={value}", content, flags=re.MULTILINE)
    else:
        if content and not content.endswith('\n'):
            content += '\n'
        content += f"{key}={value}\n"
        
    env_file.write_text(content)
    return {"status": "success", "message": f"'{key}' has been set in .env."}

def create_project_directory(project_name: str):
    """Creates a new project directory."""
    user = os.getenv('USER')
    if not user:
        raise ValueError("USER environment variable must be set to create a project directory.")
    if not project_name:
        raise ValueError("Missing 'project_name'.")
        
    (get_projects_root(user) / project_name).mkdir(exist_ok=True)
    return {"status": "success", "message": f"Project directory '{project_name}' created."}

def check_docker_socket():
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

def launch_ide(ide_name: str):
    """Launches the specified IDE service using docker compose."""
    if ide_name not in ["vnc", "vscode", "codeserver"]:
        return {"status": "error", "message": "Invalid IDE specified."}
    subprocess.run(["docker", "compose", "up", "-d", ide_name], check=True)
    return {"status": "success", "message": f"IDE '{ide_name}' is starting."}

# --- Registries ---
TOOL_REGISTRY = {
    "get_environment": get_environment,
    "get_projects": get_projects,
    "create_dot_env_file": create_dot_env_file,
    "set_environment_variable": set_environment_variable,
    "create_project_directory": create_project_directory,
    "check_docker_socket": check_docker_socket,
    "launch_ide": launch_ide
}

# --- Core Dispatch Logic ---
def get_capabilities_dict():
    """Provides a machine-readable description of the server's capabilities."""
    tool_descriptions = []
    for name, func in TOOL_REGISTRY.items():
        sig = inspect.signature(func)
        params = [{"name": p.name, "type": str(p.annotation), "required": p.default is inspect.Parameter.empty}
                  for p in sig.parameters.values()]
        tool_descriptions.append({"name": name, "description": func.__doc__.strip() if func.__doc__ else "", "parameters": params})

    return { "tools": tool_descriptions, "resources": [], "agents": [] }

def execute_tool_logic(tool_name: str, params: dict):
    tool_function = TOOL_REGISTRY.get(tool_name)
    if not tool_function: raise ValueError(f"Tool '{tool_name}' not found.")
    return tool_function(**params)

# --- Stdio Mode ---
def handle_stdio_request(req):
    req_type = req.get("type")
    try:
        if req_type == "list_all": return get_capabilities_dict()
        if req_type == "run_tool": return execute_tool_logic(req.get("name"), req.get("args", {}))
        # No more access_resource
        return {"status": "error", "message": f"unknown request type: {req_type}"}
    except Exception as e:
        return {"status": "error", "message": f"Request failed: {e}"}

def main_stdio():
    for line in sys.stdin:
        try:
            req = json.loads(line)
            resp = handle_stdio_request(req)
        except Exception as e:
            resp = {"status": "error", "message": f"An unexpected error occurred: {e}"}
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--stdio", action="store_true")
    parser.add_argument("--debug", action="store_true")
    args = parser.parse_args()
    if args.stdio:
        if args.debug: print("DEBUG mode enabled for downstream MCP server.", file=sys.stderr)
        main_stdio()