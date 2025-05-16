#!/bin/bash
set -e

# Validate required environment variables
if [ -z "$GITHUB_TOKEN" ] || [ -z "$GITHUB_USER" ] || [ -z "$GITHUB_REPO" ] || [ -z "$GITHUB_ORG" ]; then
    echo "Missing required environment variables. Ensure GITHUB_TOKEN, GITHUB_USER, GITHUB_REPO, and GITHUB_ORG are set."
    exit 1
fi

GITHUB_API="https://api.github.com"
AUTH_HEADER="Authorization: Bearer $GITHUB_TOKEN"

# Prevent .env from being committed
echo ".env" >> .gitignore

# Function to check if the repository exists
check_repo() {
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO")
    if [ "$RESPONSE" -eq 200 ]; then
        echo "Repository $GITHUB_REPO exists."
    else
        echo "Repository $GITHUB_REPO does not exist."
    fi
}

# Function to create a new repository
create_repo() {
    echo "Creating repository: $GITHUB_REPO"

    if [ "$GITHUB_ORG" == "$GITHUB_USER" ]; then
        CREATE_URL="$GITHUB_API/user/repos"
    else
        CREATE_URL="$GITHUB_API/orgs/$GITHUB_ORG/repos"
    fi

    RESPONSE=$(curl -s -H "$AUTH_HEADER" -H "Accept: application/vnd.github+json" -X POST \
        "$CREATE_URL" \
        -d "{\"name\": \"$GITHUB_REPO\", \"private\": false}")

    REPO_URL=$(echo "$RESPONSE" | jq -r '.html_url')

    if [ "$REPO_URL" != "null" ]; then
        echo "Repository created: $REPO_URL"
    else
        echo "Failed to create repository: $RESPONSE"
        exit 1
    fi
}

# Function to fetch the repo to a specified directory.
fetch_repo() {
    if [ -z "$1" ]; then
        echo "Error: No destination directory given."
        exit 1
    fi

    echo "Fetching the repo from github to $1"

    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"
    diff "$GITHUB_REPO" "$1"
    cp -r "$GITHUB_REPO"/* "$1"/ || true
}

# Function to diff the repo against the src directory
diff_repo() {
    echo "Diffing the repo against src..."

    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"

    # Copy files from the source directory
    SOURCE_DIR="/src"
    echo "Diffing repo files from source directory: $SOURCE_DIR against $GITHUB_REPO"
    diff "$SOURCE_DIR"/ "$GITHUB_REPO"/
}

# Function to create a new branch
create_branch() {
    echo "Creating a new branch..."

    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"
    cd "$GITHUB_REPO"

    if [ "$1" == "clean" ]; then
	      echo "Cleaning files in repository."
	      rm ./*
    fi	
    
    # Set Git author information (fixes missing PR author issue)
    git config --global user.name "$GITHUB_USER"
    git config --global user.email "${GITHUB_USER}@users.noreply.github.com"

    # Copy files from the source directory
    SOURCE_DIR="/src"
    echo "Copying files from source directory: $SOURCE_DIR"
    cp -r "$SOURCE_DIR"/* .

    # MAIN_BRANCH="main"
    BRANCH_NAME="auto-update-$(date +%s)"

    # git fetch origin "$MAIN_BRANCH"
    # git checkout "$MAIN_BRANCH"
    git checkout -b "$BRANCH_NAME"
    git add -u .
    git commit -m "Automated update from container"
    git push origin "$BRANCH_NAME"

    echo "Branch created: $BRANCH_NAME"
}

# Function to create a pull request
create_pull_request() {
    if [ -z "$1" ]; then
        echo "Error: No branch specified for the pull request."
        exit 1
    fi

    BRANCH_NAME="$1"

    # Get the default branch of the repository
    DEFAULT_BRANCH=$(curl -s -H "$AUTH_HEADER" "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO" | jq -r '.default_branch')

    if [ "$DEFAULT_BRANCH" == "null" ]; then
        echo "Error: Could not determine the default branch. Check if the repository exists."
        exit 1
    fi

    echo "Using default branch: $DEFAULT_BRANCH"

    # Check if the branch exists
    BRANCH_EXISTS=$(curl -s -H "$AUTH_HEADER" "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO/branches/$BRANCH_NAME" | jq -r '.name')

    if [ "$BRANCH_EXISTS" == "null" ]; then
        echo "Error: Branch '$BRANCH_NAME' does not exist in $GITHUB_REPO."
        exit 1
    fi

    # Construct correct head reference (org vs. forked repo)
    HEAD_REF="$BRANCH_NAME"  # Use only the branch name if it's within the same repo

    PR_RESPONSE=$(curl -s -H "$AUTH_HEADER" -H "Accept: application/vnd.github+json" -X POST \
        "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO/pulls" \
        -d "{
            \"title\": \"Automated Update - $(date)\",
            \"head\": \"$HEAD_REF\",
            \"base\": \"$DEFAULT_BRANCH\",
            \"body\": \"This PR was created automatically by a Docker container.\",
            \"maintainer_can_modify\": true
        }")

    PR_URL=$(echo "$PR_RESPONSE" | jq -r '.html_url')

    if [ "$PR_URL" != "null" ]; then
        echo "Pull request created successfully: $PR_URL"
    else
        echo "Failed to create pull request."
        echo "$PR_RESPONSE"
    fi
}


# Main logic to handle command-line arguments
case "$1" in
    check_repo)
        check_repo
        ;;
    create_repo)
        create_repo
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
    bash)
        exec /bin/bash
        ;;
    *)
        echo "Usage: $0 {check_repo|create_repo|diff_repo|fetch_repo <dest_dir>|create_branch <|clean>|create_pull_request <branch_name>|bash}"
        exit 1
        ;;
esac
