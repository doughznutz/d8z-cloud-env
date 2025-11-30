import logging
import os
import re
from pathlib import Path

from core.registry import MCPRegistry, Tool
from core.downstream_manager import DownstreamManager # This might not be needed for simple tools

log = logging.getLogger(__name__)

def make_tools(registry: MCPRegistry, downstream: DownstreamManager):
    tools = {}

    def get_environment(args):
        """
        Returns the runtime environment variables as a structured object.
        If a 'key' is provided, it returns the value for that key only.
        """
        key = args.get("key")
        if key:
            return {"value": os.getenv(key)}
        return dict(os.environ)

    tools["ENV_get_environment"] = Tool(
        name="ENV_get_environment",
        description=get_environment.__doc__.strip(),
        parameters=[{"name": "key", "type": "str", "required": False}],
        run_fn=get_environment
    )

    def create_dot_env_file(args):
        """Creates an empty .env file if it does not already exist."""
        Path('.env').touch(exist_ok=True)
        return {"status": "success", "message": ".env file created or already exists."}

    tools["ENV_create_dot_env_file"] = Tool(
        name="ENV_create_dot_env_file",
        description=create_dot_env_file.__doc__.strip(),
        parameters=[],
        run_fn=create_dot_env_file
    )

    def set_environment_variable(args):
        """Sets or updates a variable in the .env file."""
        key = args.get("key")
        value = args.get("value")
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

    tools["ENV_set_environment_variable"] = Tool(
        name="ENV_set_environment_variable",
        description=set_environment_variable.__doc__.strip(),
        parameters=[{"name": "key", "type": "str", "required": True}, {"name": "value", "type": "str", "required": True}],
        run_fn=set_environment_variable
    )

    return tools
