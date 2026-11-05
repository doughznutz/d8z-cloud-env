# Streamlined IDE: linuxserver/code-server + our AI agent CLIs, no VNC desktop.
FROM lscr.io/linuxserver/code-server:latest

# Base dev tooling
RUN apt-get update && apt-get install -y \
    curl wget gnupg2 ca-certificates \
    build-essential git unzip tar \
    python3 python3-pip \
    jq make docker.io \
    && rm -rf /var/lib/apt/lists/*

# docker compose plugin (host docker.sock is bind-mounted for DinD)
RUN mkdir -p /usr/libexec/docker/cli-plugins && \
    curl -fsSL https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
      -o /usr/libexec/docker/cli-plugins/docker-compose && \
    chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# Node.js (LTS) + Vite
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g vite

# Go
ENV GO_VERSION=1.26.10
RUN wget -q https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -O /tmp/go.tgz && \
    tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
ENV PATH="/usr/local/go/bin:${PATH}"

# AI agent CLIs
RUN npm install -g \
    @google/gemini-cli \
    @anthropic-ai/claude-code \
    @openai/codex \
    @continuedev/cli

# Code-server extensions. The install-extension helper needs the s6 runtime
# env which only exists once the container is running, so call the binary
# directly here (same invocation the helper uses under the hood).
RUN /app/code-server/bin/code-server --extensions-dir /config/extensions --install-extension Continue.continue && \
    /app/code-server/bin/code-server --extensions-dir /config/extensions --install-extension golang.go && \
    /app/code-server/bin/code-server --extensions-dir /config/extensions --install-extension ms-python.python

EXPOSE 8443

# Continue config defaults, pointing at the local models served by og (http://og:11434)
COPY ./continue/ /defaults/continue/
COPY ./custom-cont-init.d/ /custom-cont-init.d/
RUN chmod +x /custom-cont-init.d/*.sh

# code-server deletes GITHUB_TOKEN from its own env (it uses it for GitHub
# auth) so terminals never see it; restore it in interactive shells from the
# container env that s6 keeps in /run/s6/container_environment.
RUN echo '[ -z "$GITHUB_TOKEN" ] && [ -r /run/s6/container_environment/GITHUB_TOKEN ] && export GITHUB_TOKEN="$(cat /run/s6/container_environment/GITHUB_TOKEN)"' >> /etc/bash.bashrc
