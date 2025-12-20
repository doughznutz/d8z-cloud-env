# GitHub Environment

The `github/` directory provides a containerized environment for automating interactions with GitHub. It serves as a proxy for performing various repository and version control operations via the GitHub API and `git` commands.

## Components

*   **`Dockerfile`**: Builds an Ubuntu-based container image equipped with `git`, `curl`, and `jq`, which are essential for cloning repositories and interacting with the GitHub REST API.

*   **`entrypoint.sh`**: The primary entrypoint for the `github` container. It validates that the necessary `GITHUB_*` environment variables are set and dispatches commands to the appropriate functions. The exposed commands include:
    *   `check_repo`
    *   `create_repo`
    *   `diff_repo`
    *   `fetch_repo`
    *   `create_branch`
    *   `create_pull_request`
    *   `rebase_repo`
    *   `rename_branch`

*   **`functions.sh`**: This script contains the core logic. It defines shell functions that use `curl` to make authenticated calls to the GitHub API and `git` to perform repository operations like cloning, committing, and pushing.

*   **`github.mk`**: A Makefile that defines simple targets for executing the GitHub functions using `docker compose run`. This simplifies common CI/CD or development tasks like creating a repository (`make create_repo`) or creating a pull request (`make pull_request BRANCH=...`).
