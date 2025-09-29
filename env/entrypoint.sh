#!/bin/bash
set -e

# Usage / Help
usage_help() {
    echo "docker run \\"
    echo "  -it --rm \\"
    echo "  --env-file <(env) \\"
    echo "  -v /var/run/docker.sock:/var/run/docker.sock \\"
    echo "  -v /home/$USER/projects:/home/$USER/projects \\"
    echo "   doughznutz/doughznutz:d8z-cloud-env \\"
    echo "   {command}"
    echo
    echo "Available commands:"
    echo "  build"
}

# Check docker setup
check_docker() {
if [ -S /var/run/docker.sock ]; then
  echo "/var/run/docker.sock exists and is readable"
else
  echo "Error: /var/run/docker.sock does not exist or is not readable"
  echo "Please run: sudo chmod a+rw /var/run/docker.sock"
  usage_help
  exit 1
fi
}



# Check that an environment .env file exists.
check_env() {
if [ ! -f .env ]; then
  # Create it from the template if it doesn't.
  cp env/startup/example.env .env
fi

# Get the existing values from the environment
PROJECT=${PROJECT}
GITHUB_REPO=${GITHUB_REPO:-$PROJECT}
DOCKER_HUB_IMAGE=${DOCKER_HUB_IMAGE:-$PROJECT}

# Set values and ask user if necessary
if [ -z "$PROJECT" ]; then
  read -p "Project name: " PROJECT
fi
export PROJECT=$PROJECT
sed -i "s/^PROJECT=.*/PROJECT=$PROJECT/g" .env || echo "PROJECT=$PROJECT" >> .env

if [ -z "$GITHUB_REPO" ]; then
  read -p "GitHub repository [${PROJECT}]: " INPUT_GITHUB_REPO
  GITHUB_REPO=${INPUT_GITHUB_REPO:-$PROJECT}
fi
export GITHUB_REPO=$GITHUB_REPO
sed -i "s/^GITHUB_REPO=.*/GITHUB_REPO=$GITHUB_REPO/g" .env || echo "GITHUB_REPO=$GITHUB_REPO" >> .env

if [ -z "$DOCKER_HUB_IMAGE" ]; then
  read -p "Docker Hub repository [${PROJECT}]: " INPUT_DOCKER_HUB_IMAGE
  DOCKER_HUB_IMAGE=${INPUT_DOCKER_HUB_IMAGE:-$PROJECT}
fi
export DOCKER_HUB_IMAGE=$DOCKER_HUB_IMAGE
sed -i "s/^DOCKER_HUB_IMAGE=.*/DOCKER_HUB_IMAGE=$DOCKER_HUB_IMAGE/g" .env || echo "DOCKER_HUB_IMAGE=$DOCKER_HUB_IMAGE" >> .envfi

echo "Here is a .env file you can use",
cat .env
echo ""
}

# Check user

# Check project 
check_project() {
if [ ! -d "/home/$USER/projects" ]; then
  echo "Please mount your projects directory."
  exit 1
fi

# if $PROJECT is not set, ask the user for it.
if [ -z "${PROJECT}" ]; then
  read -p "Please enter your project name: " PROJECT
  export PROJECT
fi

# once $PROJECT is set, check /home/$USER/projects/$PROJECT exists.
while [ ! -d "/home/$USER/projects/$PROJECT" ]; do
  # if it doesnt exist, ask if we should pull from github, or if it is a new project.
  echo "/home/$USER/projects/$PROJECT does not exist."
  read -p "Do you want to pull from GitHub (g) or create a new project (n) or quit (q)? " CHOICE
  
  case $CHOICE in
    g)
      read -p "Please enter your GitHub repository URL: " REPO_URL
      git clone "$REPO_URL" "/home/$USER/projects/$PROJECT"
      ;;
    n)
      mkdir -p "/home/$USER/projects/$PROJECT"
      ;;
    q)
      exit 1
      ;;
    *)
      echo "Invalid choice. Please choose 'g' for GitHub or 'n' for new project."
      ;;
  esac
done
echo "Project looks good."
}
# Main logic to handle command-line arguments
case "$1" in
    help)
        usage_help
        exit 0
        ;;
    usage)
        usage_help
        exit 0
        ;;
    check_docker)
        check_docker
        ;;
    check_env)
        check_env
        ;;
    check_project)
        check_project
        ;;
    *)
        exec "$@"
        ;;
esac

# Fall through to bash.
exec bash