import sys

sys.path.insert(0, "/app/mcp")

import streamlit as st
from client import call_router

st.set_page_config(page_title="D8Z DOOD", layout="wide")
st.title("D8Z DOOD — Docker-out-of-Docker control plane")


def run_tool(name: str, args: dict = None):
    resp = call_router({"type": "run_tool", "name": name, "args": args or {}})
    if "error" in resp:
        raise RuntimeError(resp["error"])
    return resp.get("result")


overview_tab, project_tab, launch_tab, explorer_tab = st.tabs(
    ["Overview", "Project", "Launch", "MCP Explorer"]
)

with overview_tab:
    st.subheader("Environment")
    try:
        env_state = run_tool("ENV_get_environment") or {}
    except Exception as e:
        env_state = {}
        st.error(f"Could not read environment: {e}")

    col1, col2 = st.columns(2)
    col1.metric("USER", env_state.get("USER", "(not set)"))
    col2.metric("PROJECT", env_state.get("PROJECT", "(not set)"))

    st.subheader("Docker socket")
    try:
        docker_status = run_tool("DOCKER_check_docker_socket") or {}
        if docker_status.get("status") == "success":
            st.success(docker_status.get("message"))
        else:
            st.error(docker_status.get("message"))
    except Exception as e:
        st.error(f"Could not check docker socket: {e}")

with project_tab:
    st.subheader("Projects")
    try:
        projects = (run_tool("PROJECT_get_projects") or {}).get("projects", [])
    except Exception as e:
        projects = []
        st.error(f"Could not list projects: {e}")

    current_project = env_state.get("PROJECT")
    if projects:
        default_index = projects.index(current_project) if current_project in projects else 0
        selected = st.selectbox("Select active project", sorted(projects), index=default_index)
        if st.button("Set as active project") and selected:
            try:
                run_tool("ENV_set_environment_variable", {"key": "PROJECT", "value": selected})
                st.success(f"PROJECT set to '{selected}'. Restart dood to pick it up everywhere.")
            except Exception as e:
                st.error(f"Failed to set project: {e}")
    else:
        st.info("No projects found yet.")

    st.subheader("Create a new project")
    new_project = st.text_input("New project name")
    create_col, copy_col = st.columns(2)
    if create_col.button("Create empty project") and new_project:
        try:
            run_tool("PROJECT_create_project_directory", {"project_name": new_project})
            st.success(f"Created project '{new_project}'.")
            st.rerun()
        except Exception as e:
            st.error(f"Failed to create project: {e}")
    if copy_col.button("Create by copying this container (/self)") and new_project:
        try:
            result = run_tool("PROJECT_copy_self_to_project", {"project_name": new_project}) or {}
            if result.get("status") == "success":
                st.success(result.get("message"))
                st.rerun()
            else:
                st.error(result.get("message"))
        except Exception as e:
            st.error(f"Failed to copy project: {e}")

with launch_tab:
    mode = st.radio("Mode", ["Development", "Deployment"], horizontal=True)

    if mode == "Development":
        st.subheader("Launch an IDE")
        for ide in sorted(["deightz"]):
            if st.button(f"Launch {ide}", key=f"ide_{ide}"):
                try:
                    result = run_tool("DOCKER_launch_ide", {"ide_name": ide}) or {}
                    if result.get("status") == "success":
                        st.success(result.get("message"))
                    else:
                        st.error(result.get("message"))
                except Exception as e:
                    st.error(f"Failed to launch {ide}: {e}")
    else:
        st.subheader("Launch a deployment service")
        for service in sorted(["cloud", "docker", "github"]):
            if st.button(f"Launch {service}", key=f"svc_{service}"):
                with st.spinner(f"Launching '{service}'..."):
                    try:
                        result = run_tool("DOCKER_launch_service", {"service_name": service}) or {}
                        if result.get("status") == "success":
                            st.success(result.get("message"))
                        else:
                            st.error(result.get("message"))
                    except Exception as e:
                        st.error(f"Failed to launch {service}: {e}")

with explorer_tab:
    st.subheader("MCP Registry")
    try:
        registry = run_tool("ROUTER_list_registry") or {}
    except Exception as e:
        registry = {}
        st.error(f"Could not read registry: {e}")

    tools = registry.get("tools", [])
    resources = registry.get("resources", [])
    agents = registry.get("agents", [])

    st.caption(f"{len(tools)} tools · {len(resources)} resources · {len(agents)} agents")

    if resources:
        st.markdown("**Resources**")
        st.table([{"name": r["name"], "description": r.get("description", "")} for r in resources])
    if agents:
        st.markdown("**Agents**")
        st.table([{"name": a["name"], "description": a.get("description", "")} for a in agents])

    st.markdown("**Tools**")
    tool_names = sorted(t["name"] for t in tools)
    if tool_names:
        chosen = st.selectbox("Select a tool to run", tool_names)
        tool = next(t for t in tools if t["name"] == chosen)
        st.caption(tool.get("description", ""))

        tool_args = {}
        for param in tool.get("parameters", []):
            label = param["name"] + (" (required)" if param.get("required") else "")
            tool_args[param["name"]] = st.text_input(label, key=f"param_{chosen}_{param['name']}")

        if st.button("Run tool"):
            try:
                args = {k: v for k, v in tool_args.items() if v != ""}
                result = run_tool(chosen, args)
                st.json(result)
            except Exception as e:
                st.error(f"Tool run failed: {e}")
    else:
        st.info("No tools registered.")
