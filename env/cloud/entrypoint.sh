#!/bin/bash
set -e

# Source the functions
source /app/functions.sh

# Main command dispatcher
COMMAND=$1

if [ -z "$COMMAND" ]; then
    echo "Usage: cloud <command> [options]"
    echo "Commands:"
    echo "  create <instance-name> - Create a new instance"
    echo "  start <instance-name>  - Start an instance"
    echo "  stop <instance-name>   - Stop an instance"
    echo "  delete <instance-name> - Delete an instance"
    echo "  list                   - List all instances"
    exit 1
fi

shift # Remove command from argument list

case "$COMMAND" in
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
    *)
        echo "Unknown command: $COMMAND"
        exit 1
        ;;
esac