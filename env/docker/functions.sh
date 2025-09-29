#!/bin/bash
set -e

# Function to check if jq is installed
check_jq() {
    if ! command -v jq &> /dev/null; then
        printf "Error: jq is not installed. Please install it to use this script.\n" >&2
        exit 1
    fi
}

# Function to check if a Docker Hub repository exists
check_docker_repo() {
    local response
    local auth_string=$(printf "%s:%s" "${DOCKER_HUB_USERNAME}" "${DOCKER_HUB_TOKEN}" | base64)
    response=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Basic ${auth_string}" \
        "https://hub.docker.com/v2/repositories/${DOCKER_HUB_USERNAME}/${DOCKER_HUB_REPO}")

    if [ "${response}" -eq 200 ]; then
        printf "Docker Hub repository %s exists.\n" "${DOCKER_HUB_REPO}"
    else
        printf "Docker Hub repository %s does not exist.\n" "${DOCKER_HUB_REPO}"
    fi
}

# Function to create a new Docker Hub repository
create_docker_repo() {
    printf "Creating Docker Hub repository: %s\n" "${DOCKER_HUB_REPO}"

    local response
    local auth_string=$(printf "%s:%s" "${DOCKER_HUB_USERNAME}" "${DOCKER_HUB_TOKEN}" | base64)
    response=$(curl -s -X POST \
        -H "Authorization: Basic ${auth_string}" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"${DOCKER_HUB_REPO}\", \"description\": \"\", \"is_private\": true}" \
        "https://hub.docker.com/v2/repositories")

    local repo_url
    repo_url=$(echo "${response}" | jq -r '.url')

    if [ "${repo_url}" != "null" ]; then
        printf "Docker Hub repository created: %s\n" "${repo_url}"
    else
        printf "Failed to create Docker Hub repository: %s\n" "${response}" >&2
        exit 1
    fi
}


# Function to build and push a Docker image to Docker Hub
build_and_push_image() {
    local full_image_name="${DOCKER_HUB_USERNAME}/${DOCKER_HUB_REPO}:${DOCKER_HUB_IMAGE}"

    printf "Building and pushing Docker image: %s\n" "${full_image_name}"

    # Build the Docker image
    docker build -t "${full_image_name}" -f /path/to/Dockerfile .

    # Push the Docker image to Docker Hub
    docker login -u "${DOCKER_HUB_USERNAME}" -p "${DOCKER_HUB_TOKEN}"
    docker push "${full_image_name}"

    printf "Docker image pushed successfully.\n"
}

# Function to pull a Docker image from Docker Hub
pull_image() {
    local full_image_name="${DOCKER_HUB_USERNAME}/${DOCKER_HUB_REPO}:${DOCKER_HUB_IMAGE}"

    printf "Pulling Docker image: %s\n" "${full_image_name}"

    docker login -u "${DOCKER_HUB_USERNAME}" -p "${DOCKER_HUB_TOKEN}"
    docker pull "${full_image_name}"

    printf "Docker image pulled successfully.\n"
}

  # Function to list Docker Hub repositories for the authenticated user
  list_docker_repos() {
      printf "Listing Docker Hub repositories for user: %s\n" "${DOCKER_HUB_USERNAME}"
      local url="https://hub.docker.com/v2/repositories/${DOCKER_HUB_USERNAME}/?page_size=100"
      local auth_string=$(printf "%s:%s" "${DOCKER_HUB_USERNAME}" "${DOCKER_HUB_TOKEN}" | base64)

      while [ -n "${url}" ] && [ "${url}" != "null" ]; do
          local response
          response=$(curl -s -H "Authorization: Basic ${auth_string}" "${url}")
          echo "${response}" | jq -r '.results[].name'
          url=$(echo "${response}" | jq -r '.next')
      done
  }

  # Function to list images (tags) in a Docker Hub repository
  list_repo_images() {
      if [ -z "${DOCKER_HUB_REPO}" ]; then
          printf "Error: DOCKER_HUB_REPO environment variable is not set.\n" >&2
          exit 1
      fi

      printf "Listing images in repository: %s/%s\n" "${DOCKER_HUB_USERNAME}" "${DOCKER_HUB_REPO}"
      local url="https://hub.docker.com/v2/repositories/${DOCKER_HUB_USERNAME}/${DOCKER_HUB_REPO}/tags/?page_size=100"
      local auth_string=$(printf "%s:%s" "${DOCKER_HUB_USERNAME}" "${DOCKER_HUB_TOKEN}" | base64)

      while [ -n "${url}" ] && [ "${url}" != "null" ]; do
          local response
          response=$(curl -s -H "Authorization: Basic ${auth_string}" "${url}")
          echo "${response}" | jq -r '.results[].name'
          url=$(echo "${response}" | jq -r '.next')
      done
  }

check_jq