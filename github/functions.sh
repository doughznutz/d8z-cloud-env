#!/bin/bash

# Function to check if the repository exists
check_repo() {
    RESPONSE=$(curl -s -o /dev/null -w "%{\nhttp_code}" -H "$AUTH_HEADER" "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO")
    if [ "$RESPONSE" -eq 200 ]; then
        echo "Repository $GITHUB_REPO exists."
    else
        echo "Repository $GITHUB_REPO does not exist."
    fi
}

# Function to create a new repository
create_repo() {
    echo "Creating repository: $GITHUB_REPO"

    CREATE_URL="$GITHUB_API/orgs/$GITHUB_ORG/repos"
    if [ "$GITHUB_ORG" == "$GITHUB_USER" ]; then
        CREATE_URL="$GITHUB_API/user/repos"
    fi

    PRIVATE_REPO=true
    if [ "$1" == "public" ]; then
	    echo "Creating public repo."
        PRIVATE_REPO=false
    fi	

    RESPONSE=$(curl -s \
        -H "$AUTH_HEADER" \
        -H "Accept: application/vnd.github+json" \
        -X POST "$CREATE_URL" \
        -d "{\"name\": \"$GITHUB_REPO\", \"private\": \"$PRIVATE_REPO\"}" \
        )

    REPO_URL=$(echo "$RESPONSE" | jq -r '.html_url')

    if [ "$REPO_URL" != "null" ]; then
        echo "Repository created: $REPO_URL"
    else
        echo "Failed to create repository: $RESPONSE"
        exit 1
    fi

    # Clone the new repository
    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"
    cd "$GITHUB_REPO"

    # Create a README file, commit, and push to main
    echo "# $GITHUB_REPO" > README.md
    git config --global user.name "$GITHUB_USER"
    git config --global user.email "${GITHUB_USER}@users.noreply.github.com"
    git checkout -b main
    git add README.md
    git commit -m "Initial commit"
    git push -u origin main

    echo "Default 'main' branch created and pushed to origin."
}

# Function to fetch the repo to a specified directory.
fetch_repo() {
    if [ -z "$1" ]; then
        echo "Error: No destination directory given."
        exit 1
    fi

    echo "Fetching the repo from github to $1"

    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"
    diff  "$GITHUB_REPO" "$1"
    cp -r "$GITHUB_REPO"/* "$1"/ || true
}

# Function to diff the repo against the src directory
diff_repo() {
    echo "Diffing the repo against src..."

    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"

    # Copy files from the source directory
    SOURCE_DIR="/src"
    echo "Diffing repo files from source directory: $SOURCE_DIR against $GITHUB_REPO"
    diff -r "$SOURCE_DIR"/ "$GITHUB_REPO"/
}

# Function to create a new branch
create_branch() {
    echo "Creating a new branch..."

    git clone "https://$GITHUB_USER:$GITHUB_TOKEN@github.com/$GITHUB_ORG/$GITHUB_REPO.git"
    cd "$GITHUB_REPO"

    if [ "$1" == "clean" ]; then
	      echo "Cleaning files in repository."
	      rm -rf ./*
    fi	
    
    # Set Git author information (fixes missing PR author issue)
    git config --global user.name "$GITHUB_USER"
    git config --global user.email "${GITHUB_USER}@users.noreply.github.com"

    # Copy files from the source directory
    SOURCE_DIR="/src"
    echo "Copying files from source directory: $SOURCE_DIR"
    cp -R "$SOURCE_DIR"/* .

    # MAIN_BRANCH="main"
    BRANCH_NAME="auto-update-$(date +%s)"

    # git fetch origin "$MAIN_BRANCH"
    # git checkout "$MAIN_BRANCH"
    git checkout -b "$BRANCH_NAME"
    git add -u .
    git add --all .
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

# Function to rebase the repo
rebase_repo() {
    echo "Rebasing repository..."

    cd /src

    # Set Git author information
    git config --global user.name "$GITHUB_USER"
    git config --global user.email "${GITHUB_USER}@users.noreply.github.com"

    # Get the default branch of the repository
    DEFAULT_BRANCH=$(curl -s -H "$AUTH_HEADER" "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO" | jq -r '.default_branch')

    if [ "$DEFAULT_BRANCH" == "null" ]; then
        echo "Error: Could not determine the default branch. Check if the repository exists."
        exit 1
    fi

    echo "Default branch is: $DEFAULT_BRANCH"

    # Fetch the latest changes from origin
    git fetch origin

    # Rebase the current branch onto the default branch
    git rebase "origin/$DEFAULT_BRANCH"

    echo "Rebase complete."
}

# Function to rename a branch
rename_branch() {
    if [ -z "$1" ] || [ -z "$2" ]; then
        echo "Usage: rename_branch <old_name> <new_name>"
        exit 1
    fi

    OLD_NAME="$1"
    NEW_NAME="$2"

    echo "Renaming branch '$OLD_NAME' to '$NEW_NAME'..."

    # Rename the branch on GitHub
    RESPONSE=$(curl -s -o /dev/null -w "%{\http_code}" \
        -H "$AUTH_HEADER" \
        -H "Accept: application/vnd.github+json" \
        -X POST "$GITHUB_API/repos/$GITHUB_ORG/$GITHUB_REPO/branches/$OLD_NAME/rename" \
        -d "{\"new_name\": \"$NEW_NAME\"}")

    if [ "$RESPONSE" -eq 201 ]; then
        echo "Branch renamed successfully."
    else
        echo "Error: Failed to rename branch. Response code: $RESPONSE"
        exit 1
    fi
}
