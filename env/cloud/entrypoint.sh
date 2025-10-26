#!/bin/bash
set -e

# Source the function scripts
# The WORKDIR is /self, so we use an absolute path.
source /self/env/cloud/functions.sh

# Main command dispatcher
COMMAND=$1

if [ -z "$COMMAND" ]; then
    echo "Usage: cloud <command> [options]"
    echo ""
    echo "Instance Commands:"
    echo "  create <instance-name> - Create a new instance"
    echo "  start <instance-name>  - Start an instance"
    echo "  stop <instance-name>   - Stop an instance"
    echo "  delete <instance-name> - Delete an instance"
    echo "  list                   - List all instances"
    echo ""
    echo "Project CLI Commands:"
    echo "  list-projects          - List accessible GCP projects"
    echo "  describe-project <id>  - Describe a GCP project"
    echo "  set-project <id>       - Set the active GCP project"
    echo ""
    echo "MCP Server:"
    echo "  mcp-server             - Start the MCP HTTP server"
    exit 1
fi

shift # Remove command from argument list

case "$COMMAND" in
    # Instance commands
    create)
        create_instance "$@"
        ;;
    start)
        start_instance "$@"
        ;;
    stop)
        stop_instance "$@"
        ;;
    delete)
        delete_instance "$@"
        ;;
    list)
        list_instances "$@"
        ;;

    # Project CLI commands
    list-projects)
        list_projects "$@"
        ;;
    describe-project)
        describe_project "$@"
        ;;
    set-project)
        set_project "$@"
        ;;

    # MCP Server
    mcp-server)
        uvicorn env.cloud.mcp_server:app --host 0.0.0.0 --port 8080
        ;;

    *)
        echo "Unknown command: $COMMAND"
        exit 1
        ;;
esac