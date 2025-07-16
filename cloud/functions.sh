#!/bin/bash
set -e

# Function to check for required variables
check_vars() {
    if [ -z "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
        echo "Error: GOOGLE_APPLICATION_CREDENTIALS environment variable is not set."
        echo "This should be the path to your service account key file inside the container."
        exit 1
    fi
    if [ ! -f "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
        echo "Error: Credentials file not found at '$GOOGLE_APPLICATION_CREDENTIALS'."
        exit 1
    fi
    if [ -z "$CLOUD_PROJECT_ID" ]; then
        echo "Error: CLOUD_PROJECT_ID environment variable is not set."
        exit 1
    fi
    if [ -z "$COMPUTE_ZONE" ]; then
        echo "Error: COMPUTE_ZONE environment variable is not set."
        exit 1
    fi
}

# Function to create a new GCE instance
create_instance() {
    check_vars
    INSTANCE_NAME=$1
    if [ -z "$INSTANCE_NAME" ]; then
        echo "Usage: $0 create <instance-name>"
        exit 1
    fi
    echo "Creating instance '$INSTANCE_NAME' in project '$CLOUD_PROJECT_ID' and zone '$COMPUTE_ZONE'..."
    gcloud compute instances create "$INSTANCE_NAME" \
        --project="$CLOUD_PROJECT_ID" \
        --zone="$COMPUTE_ZONE" \
        --machine-type=e2-medium \
        --image-family=ubuntu-2004-lts \
        --image-project=ubuntu-os-cloud \
        --boot-disk-size=10GB
}

# Function to start a GCE instance
start_instance() {
    check_vars
    INSTANCE_NAME=$1
    if [ -z "$INSTANCE_NAME" ]; then
        echo "Usage: $0 start <instance-name>"
        exit 1
    fi
    echo "Starting instance '$INSTANCE_NAME'..."
    gcloud compute instances start "$INSTANCE_NAME" --project="$CLOUD_PROJECT_ID" --zone="$COMPUTE_ZONE"
}

# Function to stop a GCE instance
stop_instance() {
    check_vars
    INSTANCE_NAME=$1
    if [ -z "$INSTANCE_NAME" ]; then
        echo "Usage: $0 stop <instance-name>"
        exit 1
    fi
    echo "Stopping instance '$INSTANCE_NAME'..."
    gcloud compute instances stop "$INSTANCE_NAME" --project="$CLOUD_PROJECT_ID" --zone="$COMPUTE_ZONE"
}

# Function to delete a GCE instance
delete_instance() {
    check_vars
    INSTANCE_NAME=$1
    if [ -z "$INSTANCE_NAME" ]; then
        echo "Usage: $0 delete <instance-name>"
        exit 1
    fi
    echo "Deleting instance '$INSTANCE_NAME'..."
    gcloud compute instances delete "$INSTANCE_NAME" --project="$CLOUD_PROJECT_ID" --zone="$COMPUTE_ZONE" --quiet
}

# Function to list GCE instances
list_instances() {
    check_vars
    echo "Listing instances in project '$CLOUD_PROJECT_ID'..."
    gcloud compute instances list --project="$CLOUD_PROJECT_ID"
}
