#!/bin/bash
set -e

# Validate required environment variables
if [ -z "$GITHUB_TOKEN" ] || [ -z "$GITHUB_USER" ] || [ -z "$GITHUB_REPO" ] || [ -z "$GITHUB_ORG" ]; then
    echo "Missing required environment variables. Ensure GITHUB_TOKEN, GITHUB_USER, GITHUB_REPO, and GITHUB_ORG are set."
    exit 1
fi

GITHUB_API="https://api.github.com"
AUTH_HEADER="Authorization: Bearer $GITHUB_TOKEN"

# Source the functions
. /self/deploy/github/functions.sh

# Prevent .env from being committed
echo ".env" >> .gitignore

# Main logic to handle command-line arguments
case "$1" in
    help)
        echo "Usage: $0 {command}"
        echo "Available commands:"
        echo "  check_repo"
        echo "  create_repo <description>"
        echo "  diff_repo"
        echo "  fetch_repo <dest_dir>"
        echo "  create_branch <branch_name>"
        echo "  create_pull_request <branch_name>"
        echo "  rebase_repo"
        echo "  rename_branch <old_name> <new_name>"
        ;;
    check_repo)
        check_repo
        ;;
    create_repo)
        create_repo "$2"
        ;;
    diff_repo)
        diff_repo
        ;;
    fetch_repo)
        if [ -z "$2" ]; then
            echo "Usage: $0 fetch_repo <dest_dir>"
            exit 1
        fi
        fetch_repo "$2"
        ;;
    create_branch)
        create_branch "$2"
        ;;
    create_pull_request)
        if [ -z "$2" ]; then
            echo "Usage: $0 create_pull_request <branch_name>"
            exit 1
        fi
        create_pull_request "$2"
        ;;
    rebase_repo)
        rebase_repo
        ;;
    rename_branch)
        if [ -z "$2" ] || [ -z "$3" ]; then
            echo "Usage: $0 rename_branch <old_name> <new_name>"
            exit 1
        fi
        rename_branch "$2" "$3"
        ;;
    *)
        exec "$@"
        ;;
esac
