#
# FILE: ~/docker/tools/vscode/Dockerfile

# Use a prebuilt image and add stuff to it.
FROM tools:base
# FROM ubuntu:22.04

# Install dependencies for adding the repo
# Consider a fetcher if things take too long to download.
RUN apt-get update
RUN apt-get install -y \
        clang \
        cmake \
        curl \
        ninja-build \
        android-tools-adb \
        openjdk-17-jdk \
        pkg-config \
        libgtk-3-dev \
        supervisor \
        wget gnupg


# Install code-server
RUN curl -fsSL https://github.com/coder/code-server/releases/download/v4.17.1/code-server_4.17.1_amd64.deb -o /tmp/code-server.deb && \
    apt install -y /tmp/code-server.deb && \
    rm /tmp/code-server.deb

# Add the Microsoft GPG key and repository (same as above, but inside the container)
RUN wget -qO- https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor -o /usr/share/keyrings/packages.microsoft.gpg
RUN echo "deb [signed-by=/usr/share/keyrings/packages.microsoft.gpg] https://packages.microsoft.com/repos/vscode stable main" \
    | tee -a /etc/apt/sources.list.d/vscode.list

# Install Microsoft code package to allow for flutter extension.
RUN apt-get update && apt-get install -y code

# Install the Flutter extension
RUN HOME=/home/doughznutz code-server --install-extension dart-code.flutter
RUN HOME=/home/doughznutz code-server --install-extension Continue.continue

# Expose the port that code-server listens on.
EXPOSE 8080

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]