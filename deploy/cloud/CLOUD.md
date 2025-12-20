# Cloud Environment (GCP)

The `cloud/` directory provides a dedicated container environment for interacting with the Google Cloud Platform (GCP). It acts as a proxy for executing `gcloud` commands to manage cloud resources.

This environment provides three main capabilities:
1.  **GCE Instance Management**: CLI commands to manage Google Compute Engine instances.
2.  **GCP Project Management**: CLI commands to manage Google Cloud projects.
3.  **MCP Server**: An HTTP server that exposes project management functions to an AI agent.

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

## 3. MCP (Model Context Protocol) Server

The MCP server is a simple HTTP server that allows an AI agent to manage GCP projects by making API calls.

*   **Command to start server**: `docker compose run -p 8080:8080 cloud mcp-server`
*   **Server listens on port**: `8080` (inside the container)

### API Endpoints

All endpoints are `POST` requests.

*   **`/list-projects`**
    *   Lists all Google Cloud projects the service account can access.
    *   **Body**: (empty)
    *   **Example `curl`**: `curl -X POST http://localhost:8080/list-projects`

*   **`/describe-project`**
    *   Shows detailed information about a specific project.
    *   **Body**: `{ "project_id": "your-gcp-project-id" }`
    *   **Example `curl`**: `curl -X POST -H "Content-Type: application/json" -d '{"project_id": "my-proj"}' http://localhost:8080/describe-project`

*   **`/set-project`**
    *   Sets the active GCP project for subsequent `gcloud` commands within the container.
    *   **Body**: `{ "project_id": "your-gcp-project-id" }`
    *   **Example `curl`**: `curl -X POST -H "Content-Type: application/json" -d '{"project_id": "my-proj"}' http://localhost:8080/set-project`

---

## Technical Components

*   **`Dockerfile`**: Builds a container image based on Ubuntu that includes the Google Cloud SDK (`gcloud`), Python, and other necessary utilities.
*   **`mcp_server.py`**: The Python script for the MCP server.
*   **`entrypoint.sh`**: The main command dispatcher for the `cloud` container. It handles all CLI commands and can start the MCP server.
*   **`functions.sh`**: This file contains all the underlying shell functions for both GCE instance and GCP project management.
*   **`README.md`**: Provides instructions on setting up GCP authentication, which is required for all functionality.

