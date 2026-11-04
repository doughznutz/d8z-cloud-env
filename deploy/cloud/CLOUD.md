# Cloud Environment (GCP)

The `cloud/` directory provides a dedicated container environment for interacting with the Google Cloud Platform (GCP). It acts as a proxy for executing `gcloud` commands to manage cloud resources.

This environment provides two main capabilities:
1.  **GCE Instance Management**: CLI commands to manage Google Compute Engine instances.
2.  **GCP Project Management**: CLI commands to manage Google Cloud projects.

Both are also reachable as "Deployment" jobs in gw-server's console (`dev/traefik/server/jobs.go`), which runs these same commands via `docker compose run`.

---

## 1. GCE Instance Management (CLI)

The following commands are available for managing GCE instances.

*   `create <instance-name>`
*   `start <instance-name>`
*   `stop <instance-name>`
*   `delete <instance-name>`
*   `list`

**Example:**
```sh
docker compose run cloud create my-test-instance
```

---

## 2. GCP Project Management (CLI)

The following commands are available for managing GCP projects.

*   `list-projects`: Lists accessible GCP projects.
*   `describe-project <project-id>`: Describes a specific project.
*   `set-project <project-id>`: Sets the active project for the `gcloud` CLI in the container.

**Example:**
```sh
docker compose run cloud list-projects
```

---

## Technical Components

*   **`Dockerfile`**: Builds a container image based on Ubuntu that includes the Google Cloud SDK (`gcloud`), Python, and other necessary utilities.
*   **`entrypoint.sh`**: The main command dispatcher for the `cloud` container. It handles all CLI commands.
*   **`functions.sh`**: This file contains all the underlying shell functions for both GCE instance and GCP project management.
*   **`README.md`**: Provides instructions on setting up GCP authentication, which is required for all functionality.

