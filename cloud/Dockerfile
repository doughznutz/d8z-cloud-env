FROM ubuntu:latest

# Update package lists
RUN apt-get update && apt-get upgrade -y

# Install necessary tools: gcloud CLI, python3, pip, git, curl, and jq
RUN apt-get install -y google-cloud-cli python3 python3-pip git curl jq

# Install google-api-python-client and google-auth-httplib2
RUN pip3 install --no-cache-dir google-api-python-client google-auth-httplib2

# Optional: Install other helpful tools (e.g., vim, less)
RUN apt-get install -y vim less

# Configure gcloud (optional, but recommended)
# You can set the project, zone, and region here, or do it at runtime
ENV CLOUD_PROJECT_ID=your-project-id
ENV COMPUTE_ZONE=your-compute-zone
ENV COMPUTE_REGION=your-compute-region

# Initialize gcloud and authenticate
RUN gcloud config set-project $CLOUD_PROJECT_ID  && \
    gcloud config set-zone $COMPUTE_ZONE && \
    gcloud config set-region $COMPUTE_REGION && \
    gcloud auth activate-service-account --key-file=/path/to/your/service_account.json

# Set the working directory
WORKDIR /app

# Copy any application code into the container (optional)
COPY . /app

# Entrypoint - you can customize this to run a specific script
ENTRYPOINT ["/app/entrypoint.sh"]