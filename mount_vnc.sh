#!/bin/bash

# Script to run a Docker container with a noVNC server, mounting the current directory.

# Docker image to use
DOCKER_IMAGE="dorowsz/alpine-desktop-x11vnc"
CONTAINER_NAME="novnc_container"
NOVNC_PORT="6080"
MOUNT_DIR="$PWD" # Mount the current working directory

# Check if the VNC_PASSWORD environment variable is set
if [ -z "$VNC_PASSWORD" ]; then
  echo "Error: The VNC_PASSWORD environment variable must be set."
  echo "Usage: VNC_PASSWORD=<your_password> ./run_vnc"
  exit 1
fi

# Pull the Docker image if it doesn't exist locally
docker pull "$DOCKER_IMAGE"

# Check if a container with the same name is already running and remove it
if docker ps --filter "name=$CONTAINER_NAME" | grep -q "$CONTAINER_NAME"; then
  echo "Warning: Container '$CONTAINER_NAME' is already running. Stopping and removing..."
  docker stop "$CONTAINER_NAME"
  docker rm "$CONTAINER_NAME"
fi

# Run the Docker container, mounting the current directory to /host_dir inside the container
echo "Starting Docker container '$CONTAINER_NAME'..."
docker run -d \
  --name "$CONTAINER_NAME" \
  -p "$NOVNC_PORT":"$NOVNC_PORT" \
  -v "$MOUNT_DIR:/host_dir" \
  -e VNC_PASSWORD="$VNC_PASSWORD" \
  "$DOCKER_IMAGE"

# Get the IP address of the Docker host
DOCKER_HOST_IP=$(ip addr show docker0 | grep -oP 'inet \K[\d.]+')
if [ -z "$DOCKER_HOST_IP" ]; then
  DOCKER_HOST_IP="localhost"
  echo "Warning: Could not automatically determine Docker host IP. Using 'localhost'."
fi

# Print the URL to access noVNC
echo ""
echo "noVNC server is running. Open the following URL in your web browser:"
echo "http://${DOCKER_HOST_IP}:${NOVNC_PORT}/vnc.html"
echo ""
echo "The current directory '$MOUNT_DIR' is mounted inside the container at '/host_dir'."
echo "You can stop the container using: docker stop $CONTAINER_NAME"
echo "You can remove the container using: docker rm $CONTAINER_NAME"

exit 0
