#!/bin/bash

# Script to run a Docker container with a noVNC server, mounting the current directory.

# Docker image to use
DOCKER_IMAGE="dorowu/ubuntu-desktop-lxde-vnc"
CONTAINER_NAME="novnc_container"
MOUNT_DIR="." # Mount the current working directory

# Default VNC server port inside the container
DEFAULT_VNC_SERVER_PORT="80"

# Check if the VNC_PASSWORD environment variable is set
if [ -z "$VNC_PASSWORD" ]; then
  echo "Error: The VNC_PASSWORD environment variable must be set."
  echo "Usage: VNC_PASSWORD=<your_password> ./run_vnc"
  exit 1
fi

# Get the VNC server port from the environment variable, or use the default
VNC_SERVER_PORT="${VNC_SERVER_PORT_INTERNAL:-$DEFAULT_VNC_SERVER_PORT}"

# Check if the NOVNC_PORT environment variable is set, otherwise use the default
if [ -z "$NOVNC_PORT" ]; then
  NOVNC_PORT="6080"
fi

# Pull the Docker image if it doesn't exist locally
docker pull "$DOCKER_IMAGE"

# Check if a container with the same name is already running and remove it
if docker ps -a --filter "name=$CONTAINER_NAME" | grep -q "$CONTAINER_NAME"; then
  echo "Warning: Container '$CONTAINER_NAME' is already running. Stopping and removing..."
  docker stop "$CONTAINER_NAME"
  docker rm "$CONTAINER_NAME"
fi

# Run the Docker container, mounting the current directory and setting environment variables
echo "Starting Docker container '$CONTAINER_NAME'..."
docker run -d \
  --name "$CONTAINER_NAME" \
  -p "$NOVNC_PORT":"$VNC_SERVER_PORT" \
  -e VNC_PASSWORD="$VNC_PASSWORD" \
  "$DOCKER_IMAGE"

#  -v "$MOUNT_DIR:/root/host_dir" \
#  -e VNC_PORT="$VNC_SERVER_PORT" \

# Get the IP address of the Docker host
# DOCKER_HOST_IP=$(ip addr show docker0 | grep -oP 'inet \K[\d.]+')
if [ -z "$DOCKER_HOST_IP" ]; then
  DOCKER_HOST_IP="localhost"
  echo "Warning: Could not automatically determine Docker host IP. Using 'localhost'."
fi

# Print the URL to access noVNC
echo ""
echo "noVNC server is running. Open the following URL in your web browser:"
echo "http://${DOCKER_HOST_IP}:${NOVNC_PORT}/vnc.html"
echo ""
echo "The VNC server inside the container is running on port: $VNC_SERVER_PORT."
echo "The current directory '$MOUNT_DIR' is mounted inside the container at '/host_dir'."
echo "You can stop the container using: docker stop $CONTAINER_NAME"
echo "You can remove the container using: docker rm $CONTAINER_NAME"
