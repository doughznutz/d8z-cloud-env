# FROM dorowu/ubuntu-desktop-lxde-vnc:focal
# FROM env:base
FROM dev:vnc

# Install essential packages
RUN apt-get update && apt-get install -y \
    curl wget gnupg2 software-properties-common build-essential

RUN apt-get update && apt-get install -y \
    git unzip tar python3 python3-pip ca-certificates

RUN apt-get update && apt-get install -y \
    gnome-keyring apt-transport-https software-properties-common

# Install Node.js (LTS) and Vite
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && \
    apt-get install -y nodejs && npm install -g vite

# Install Go
ENV GO_VERSION=1.22.3
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:$PATH"

# Install desktop VS Code (stable)
RUN curl https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor > microsoft.gpg && \
    install -o root -g root -m 644 microsoft.gpg /etc/apt/trusted.gpg.d/ && \
    sh -c 'echo "deb [arch=amd64] https://packages.microsoft.com/repos/vscode stable main" > /etc/apt/sources.list.d/vscode.list' && \
    apt-get update && apt-get install -y code && \
    rm microsoft.gpg

# Tell vscode to not worry about WSL.
ENV DONT_PROMPT_WSL_INSTALL=IS_THIS_NECESSARY

    
# Add extensions for VSCODE and Server .. These should be in project specific Dockerfiles.
USER doughznutz
ENV HOME=/home/doughznutz
RUN code --install-extension Continue.continue \
    && code --install-extension dsznajder.es7-react-js-snippets \
    && code --install-extension esbenp.prettier-vscode \
    && code --install-extension dbaeumer.vscode-eslint \
    && code --install-extension bradlc.vscode-tailwindcss \
    && code --install-extension antfu.vite \
    && code --install-extension golang.go

USER root

# Expose the port (if applicable; adjust as needed)
EXPOSE 80
EXPOSE 8080

COPY config.yaml /home/doughznutz/.continue/config.yaml

# Entrypoint for the container
# COPY *.conf /etc/supervisor/conf.d/
# CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]