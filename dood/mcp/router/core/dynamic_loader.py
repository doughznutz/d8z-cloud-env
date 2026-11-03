import logging
import importlib
import importlib.util
import sys
import time
import threading
from pathlib import Path

from .registry import MCPRegistry
from .downstream_manager import DownstreamManager

log = logging.getLogger(__name__)

class DynamicLoader:
    def __init__(self, registry: MCPRegistry, downstream_manager: DownstreamManager, tools_dir: Path, resources_dir: Path, agents_dir: Path):
        self.registry = registry
        self.downstream_manager = downstream_manager
        self.tools_dir = tools_dir
        self.resources_dir = resources_dir
        self.agents_dir = agents_dir
        self._loaded_modules = {}
        self._file_mtimes = {}
        self._tool_map = {} # module_name -> list of tool names
        self._resource_map = {} # module_name -> list of resource names
        self._agent_map = {} # module_name -> list of agent names

    def load_components(self):
        """Loads all components (tools, resources, agents) from their respective directories."""
        self._load_directory_components("tool", self.tools_dir, "make_tools", self.registry.register_tool, self._tool_map)
        self._load_directory_components("resource", self.resources_dir, "make_resources", self.registry.register_resource, self._resource_map)
        self._load_directory_components("agent", self.agents_dir, "make_agents", self.registry.register_agent, self._agent_map)

    def _load_directory_components(self, component_type: str, directory_path: Path, make_function_name: str, register_function, component_map: dict):
        if not directory_path.is_dir():
            log.warning(f"{component_type.capitalize()} directory not found: {directory_path}")
            return
        
        if str(directory_path.parent) not in sys.path:
            sys.path.insert(0, str(directory_path.parent))

        for py_file in directory_path.glob("*.py"):
            if py_file.name.startswith("__"):
                continue
            self._load_module(component_type, directory_path.name, py_file, make_function_name, register_function, component_map)

    def _load_module(self, component_type: str, directory_name: str, py_file: Path, make_function_name: str, register_function, component_map: dict):
        module_name = f"{directory_name}.{py_file.stem}"
        
        try:
            spec = importlib.util.spec_from_file_location(module_name, py_file)
            if not spec or not spec.loader:
                return

            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            
            components_in_module = []
            if hasattr(module, make_function_name):
                make_function = getattr(module, make_function_name)
                components = make_function(self.registry, self.downstream_manager)
                for name, component in components.items():
                    register_function(name, component)
                    components_in_module.append(name)
                    log.info(f"Loaded {component_type}: {name}")
            
            self._loaded_modules[module_name] = module
            self._file_mtimes[py_file] = py_file.stat().st_mtime
            component_map[module_name] = components_in_module

        except Exception as e:
            log.error(f"Error loading {component_type} from {py_file}", exc_info=e)

    def watch_for_changes(self):
        """Monitors the component directories for changes and reloads them."""
        def watch_loop():
            while True:
                self.check_and_reload_components()
                time.sleep(2)
        
        thread = threading.Thread(target=watch_loop, daemon=True)
        thread.start()
        log.info("Component hot-reloading enabled.")

    def check_and_reload_components(self):
        """Checks for modified or new component files and reloads them."""
        self._check_and_reload_directory_components("tool", self.tools_dir, "make_tools", self.registry.register_tool, self.registry.remove_tool, self._tool_map)
        self._check_and_reload_directory_components("resource", self.resources_dir, "make_resources", self.registry.register_resource, self.registry.remove_resource, self._resource_map)
        self._check_and_reload_directory_components("agent", self.agents_dir, "make_agents", self.registry.register_agent, self.registry.remove_agent, self._agent_map)
    
    def _check_and_reload_directory_components(self, component_type: str, directory_path: Path, make_function_name: str, register_function, remove_function, component_map: dict):
        if not directory_path.is_dir():
            return

        for py_file in directory_path.glob("*.py"):
            if py_file.name.startswith("__"):
                continue

            current_mtime = py_file.stat().st_mtime
            module_name = f"{directory_path.name}.{py_file.stem}"
            
            if py_file not in self._file_mtimes:
                log.info(f"New {component_type} file detected: {py_file.name}")
                self._load_module(component_type, directory_path.name, py_file, make_function_name, register_function, component_map)
            elif self._file_mtimes[py_file] < current_mtime:
                log.info(f"{component_type.capitalize()} file modified: {py_file.name}. Reloading...")
                self._reload_module(component_type, directory_path.name, py_file, make_function_name, register_function, remove_function, component_map)
    
    def _reload_module(self, component_type: str, directory_name: str, py_file: Path, make_function_name: str, register_function, remove_function, component_map: dict):
        module_name = f"{directory_name}.{py_file.stem}"

        # Unregister old components from this module
        if module_name in component_map:
            for comp_name in component_map[module_name]:
                remove_function(comp_name)
                log.info(f"Unloaded {component_type}: {comp_name}")
            del component_map[module_name]

        # Reload the module
        if module_name in self._loaded_modules:
            try:
                module = self._loaded_modules[module_name]
                importlib.reload(module)

                # Re-register components
                components_in_module = []
                if hasattr(module, make_function_name):
                    make_function = getattr(module, make_function_name)
                    components = make_function(self.registry, self.downstream_manager)
                    for name, component in components.items():
                        register_function(name, component)
                        components_in_module.append(name)
                        log.info(f"Reloaded {component_type}: {name}")
                
                component_map[module_name] = components_in_module
                self._file_mtimes[py_file] = py_file.stat().st_mtime
            except Exception as e:
                log.error(f"Error reloading {component_type} module {module_name}", exc_info=e)

