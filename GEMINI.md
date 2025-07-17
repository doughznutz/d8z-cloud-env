# Gemini Notes

This file is used by the Gemini CLI to store notes and context about this project.

## Project Context

*   The project root is `/home/doughznutz/projects/d8z-cloud-env`.
*   This `GEMINI.md` file was created at the user's request on 2025-07-03.
*   The initial interaction focused on establishing the location and purpose of this file.
*   `GEMINI.md` is updated by asking to "update the project notes".
*   The user prefers `docker compose` over `docker-compose`.
*   **Project Summary:** This project is a comprehensive, containerized development environment managed by Docker Compose. It is designed to support a wide range of development tasks, with a strong focus on two main areas: 1) Local AI/LLM development, providing tools like Ollama and LiteLLM. 2) Android and Flutter application development, including the Android SDK, Android Studio, and an emulator. The environment also offers various access methods, including web-based VS Code (code-server), a VNC remote desktop, and a custom dashboard.

## Docker Service Summaries

### Main Environment (`docker-compose.yaml`)
*   **base:** Provides the foundational image and environment for the main set of services. It includes Docker-in-Docker support.
*   **cloud:** Used for interacting with cloud provider services, configured with credentials mounted into the container.
*   **docker:** A dedicated container providing Docker-in-Docker capabilities, allowing other containers to run Docker commands.
*   **github:** Designed for GitHub operations, likely for CI/CD or managing repositories, with access to the full project context.
*   **dashboard:** A Go-based web application that serves as the main entry point for accessing the various development tools.
*   **dozzle:** A real-time log viewer for other Docker containers, providing an easy way to monitor the application stack.
*   **voideditor:** A containerized desktop environment accessible via VNC, featuring the "Void" editor with built-in AI agent capabilities.
*   **vnc:** A general-purpose VNC container providing a remote desktop, pre-configured with the Gemini CLI for AI interaction.
*   **vscode:** A container providing a VNC-accessible instance of VS Code, enhanced with the "Continue" AI agent for code assistance.
*   **codeserver:** Provides a web-based version of VS Code (code-server), allowing development from a browser, configured for external access.
*   **ollama:** Acts as a local AI gateway, running a custom Go application to serve and proxy requests to large language models.
*   **litellm:** Provides a unified interface to various LLMs (local and remote), acting as a proxy and translation layer. It uses `geminidb` for logging.
*   **gemini:** A container specifically for running the Gemini CLI, likely for interacting with Google's Gemini models through the `litellm` proxy.
*   **adminer:** A web-based database management tool for viewing and editing the contents of the project's databases.
*   **ollamadb:** A PostgreSQL database instance used for storing logs and data from the `ollama` service.
*   **geminidb:** A PostgreSQL database instance used for storing logs and data from the `litellm` and `gemini` services.

### Android Environment (`android/docker-compose.yaml`)
*   **base:** Provides the foundational image for the Android development environment, including a user, VNC server, and shared volumes for SDKs.
*   **flutter:** A container specifically for Flutter development, built on the Android `base` image.
*   **android_studio:** A container running a full instance of Android Studio, accessible via VNC.
*   **android_sdk:** A container that likely manages and provides the Android SDK tools to other containers.
*   **android_emulator:** Runs an Android emulator, using KVM for hardware acceleration.
*   **android_emulator_ots:** An alternative, pre-configured Android emulator service using a public, "off-the-shelf" Docker image.
*   **vscode:** A `code-server` instance specifically tailored for the Android development environment, providing a web-based IDE.
