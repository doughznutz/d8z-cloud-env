import logging
import os
from pathlib import Path

from core.registry import MCPRegistry, Tool
from core.downstream_manager import DownstreamManager # This might not be needed for simple tools

log = logging.getLogger(__name__)

# Helper function, not exposed as a tool
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

def make_tools(registry: MCPRegistry, downstream: DownstreamManager):
    tools = {}

    def get_projects(args):
        """
        Lists the directories in the user's project folder, based on the USER environment variable.
        """
        user = os.getenv('USER')
        if not user:
            raise ValueError("USER environment variable must be set to locate projects directory.")
            
        projects_root = get_projects_root(user)
        subdirs = [d.name for d in projects_root.iterdir() if d.is_dir()]
        return {"projects": subdirs}

    tools["PROJECT_get_projects"] = Tool(
        name="PROJECT_get_projects",
        description=get_projects.__doc__.strip(),
        parameters=[],
        run_fn=get_projects
    )

    def create_project_directory(args):
        """Creates a new project directory."""
        project_name = args.get("project_name")
        user = os.getenv('USER')
        if not user:
            raise ValueError("USER environment variable must be set to create a project directory.")
        if not project_name:
            raise ValueError("Missing 'project_name'.")
            
        (get_projects_root(user) / project_name).mkdir(exist_ok=True)
        return {"status": "success", "message": f"Project directory '{project_name}' created."}

    tools["PROJECT_create_project_directory"] = Tool(
        name="PROJECT_create_project_directory",
        description=create_project_directory.__doc__.strip(),
        parameters=[{"name": "project_name", "type": "str", "required": True}],
        run_fn=create_project_directory
    )

    return tools
