import os
import shutil
import subprocess
from typing import Optional, Dict, Any

# ==============================================================================
# Helper Functions
# ==============================================================================

def get_projects_root() -> str:
    """Constructs the path to the projects directory using the USER env var."""
    user = os.getenv("USER")
    if not user:
        raise ValueError("USER environment variable is not set inside the container.")
    return os.path.join("/home", user, "projects")

# ==============================================================================
# Tool Implementations
# ==============================================================================

def check_user_configuration(**kwargs: Any) -> Dict[str, Any]:
    """Checks if the USER environment variable is set."""
    user = os.getenv("USER")
    if user:
        return {"status": "success", "message": f"USER is configured as '{user}'."}
    
    if not os.path.exists('.env'):
        return {"status": "error", "error_code": "ENV_FILE_MISSING", "message": ".env file not found."}
    else:
        return {"status": "error", "error_code": "USER_VAR_MISSING", "message": "The USER variable is not set in the .env file."}

def check_docker_socket(**kwargs: Any) -> Dict[str, Any]:
    """Checks if the Docker socket is accessible."""
    socket_path = "/var/run/docker.sock"
    if os.path.exists(socket_path) and os.access(socket_path, os.R_OK | os.W_OK):
        return {"status": "success", "message": f"Docker socket '{socket_path}' is accessible."}
    else:
        return {"status": "error", "message": f"Docker socket '{socket_path}' is not accessible."}

def check_projects_root_directory(**kwargs: Any) -> Dict[str, Any]:
    """Checks if the main /home/{USER}/projects directory is mounted and exists."""
    projects_root = get_projects_root()
    if os.path.isdir(projects_root):
        return {"status": "success", "message": f"Projects root directory '{projects_root}' exists."}
    else:
        return {"status": "error", "message": f"The main projects directory '{projects_root}' does not exist."}

def list_projects(**kwargs: Any) -> Dict[str, Any]:
    """Lists subdirectories in the /home/{USER}/projects directory."""
    projects_root = get_projects_root()
    if not os.path.isdir(projects_root):
        raise FileNotFoundError(f"Projects root directory '{projects_root}' not found.")
    
    subdirs = [d for d in os.listdir(projects_root) if os.path.isdir(os.path.join(projects_root, d))]
    return {"status": "success", "projects": subdirs}

def create_env_file(**kwargs: Any) -> Dict[str, Any]:
    """Creates the .env file from the example template."""
    if os.path.exists('.env'):
        return {"status": "skipped", "message": ".env file already exists."}
    shutil.copyfile('env/startup/example.env', '.env')
    return {"status": "success", "message": ".env file created successfully from template."}

def update_env_var(key: str, value: str, **kwargs: Any) -> Dict[str, Any]:
    """Adds or updates a key-value pair in the .env file."""
    lines = []
    key_found = False
    if os.path.exists('.env'):
        with open('.env', 'r') as f:
            lines = f.readlines()
    
    with open('.env', 'w') as f:
        for line in lines:
            if line.strip().startswith(f"{key}="):
                f.write(f"{key}={value}\n")
                key_found = True
            else:
                f.write(line)
        if not key_found:
            f.write(f"{key}={value}\n")

    return {"status": "success", "message": f"'{key}' updated in .env file."}

def create_project_directory(project_name: str, **kwargs: Any) -> Dict[str, Any]:
    """Creates a new project directory."""
    projects_root = get_projects_root()
    project_path = os.path.join(projects_root, project_name)
    os.makedirs(project_path, exist_ok=True)
    return {"status": "success", "message": f"Project directory '{project_path}' created."}

def git_clone(repo_url: str, project_name: str, **kwargs: Any) -> Dict[str, Any]:
    """Clones a Git repository into the projects directory."""
    projects_root = get_projects_root()
    project_path = os.path.join(projects_root, project_name)
    command = ["git", "clone", repo_url, project_path]
    subprocess.run(command, check=True, capture_output=True, text=True)
    return {"status": "success", "message": f"Repository '{repo_url}' cloned to '{project_path}'."}

def get_env_var(key: str, **kwargs: Any) -> Dict[str, Any]:
    """Reads a specific variable from the .env file."""
    if not os.path.exists('.env'):
        return {"status": "error", "error_code": "ENV_FILE_MISSING"}
    
    with open('.env', 'r') as f:
        for line in f:
            line = line.strip()
            if line.startswith(f"{key}="):
                return {"status": "success", "value": line.split('=', 1)[1]}
    return {"status": "error", "error_code": "VAR_NOT_FOUND"}

def check_project_directory_exists(project_name: str, **kwargs: Any) -> Dict[str, Any]:
    """Checks if a specific project directory exists."""
    projects_root = get_projects_root()
    project_path = os.path.join(projects_root, project_name)
    if os.path.isdir(project_path):
        return {"status": "success", "message": f"Project directory '{project_path}' exists."}
    else:
        return {"status": "error", "message": f"Project directory '{project_path}' does not exist."}

def launch_ide(ide_name: str, **kwargs: Any) -> Dict[str, Any]:
    """Builds and starts a specific IDE service using docker compose."""
    # Mapping of simple names to docker-compose service names
    ide_service_map = {
        "terminal": "env", # This service is already running, but we can treat it as a no-op
        "vnc": "vnc",
        "vscode": "vscode",
        "codeserver": "codeserver"
    }
    service_name = ide_service_map.get(ide_name)
    if not service_name:
        raise ValueError(f"Invalid IDE name '{ide_name}'.")

    if service_name == "env":
        return {"status": "success", "message": "Terminal is already running. You can attach a new shell."}
        
    command = ["docker", "compose", "up", "-d", "--build", service_name]
    result = subprocess.run(command, capture_output=True, text=True)
    
    if result.returncode == 0:
        return {"status": "success", "message": f"Successfully launched '{ide_name}'.", "output": result.stdout}
    else:
        return {"status": "error", "message": f"Failed to launch '{ide_name}'.", "error": result.stderr}

# ==============================================================================
# Tool Registry
# ==============================================================================

TOOL_REGISTRY = {
    "check_user_configuration": check_user_configuration,
    "check_docker_socket": check_docker_socket,
    "check_projects_root_directory": check_projects_root_directory,
    "list_projects": list_projects,
    "create_env_file": create_env_file,
    "update_env_var": update_env_var,
    "create_project_directory": create_project_directory,
    "git_clone": git_clone,
    "get_env_var": get_env_var,
    "check_project_directory_exists": check_project_directory_exists,
    "launch_ide": launch_ide,
}
