#!/bin/bash
set -e

# Validate required environment variables
if [ -z "$GITHUB_TOKEN" ] || [ -z "$GITHUB_USER" ] || [ -z "$GITHUB_REPO" ] || [ -z "$GITHUB_ORG" ]; then
    echo "Missing required environment variables. Ensure GITHUB_TOKEN, GITHUB_USER, GITHUB_REPO, and GITHUB_ORG are set."
    exit 1
fi

GITHUB_API="https://api.github.com"
AUTH_HEADER="Authorization: Bearer $GITHUB_TOKEN"

# Function to check if the repository exists
check_repo() {
    curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO"
}

# Function to create a new repository
create_repo() {
    echo "Creating repository: $GITHUB_REPO"

    # Determine if creating under an organization or a personal account
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

# Check if the repo exists, create it if it doesn't
REPO_STATUS=$(check_repo)
if [ "$REPO_STATUS" -ne 200 ]; then
    echo "Repository $GITHUB_REPO not found. Creating..."
    create_repo
else
    echo "Repository $GITHUB_REPO already exists."
fi

# Clone the repository
echo "Cloning repository..."
git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"
cd "$GITHUB_REPO"

# Set Git author information (fixes missing PR author issue)
git config --global user.name "$GITHUB_USER"
git config --global user.email "${GITHUB_USER}@users.noreply.github.com"

# Copy files from the source directory
SOURCE_DIR="/src"
echo "Copying files from mounted directory: $SOURCE_DIR"
cp -r "$SOURCE_DIR"/* .

# Create a new branch
BRANCH_NAME="auto-update-$(date +%s)"
git checkout -b "$BRANCH_NAME"
git add .
git commit -m "Automated update from container"
git push origin "$BRANCH_NAME"

# Create a pull request
echo "Creating pull request..."
PR_RESPONSE=$(curl -s -H "$AUTH_HEADER" -H "Accept: application/vnd.github+json" -X POST \
    "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO/pulls" \
    -d "{
        \"title\": \"Automated Update\",
        \"head\": \"$BRANCH_NAME\",
        \"base\": \"main\",
        \"body\": \"This PR was created automatically by a Docker container.\",
        \"maintainer_can_modify\": true
    }")

PR_URL=$(echo "$PR_RESPONSE" | jq -r '.html_url')
if [ "$PR_URL" != "null" ]; then
    echo "Pull request created: $PR_URL"
else
    echo "Failed to create pull request."
    echo "$PR_RESPONSE"
fi
