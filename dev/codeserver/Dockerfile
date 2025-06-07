# FROM dorowu/ubuntu-desktop-lxde-vnc:focal
FROM ubuntu:latest

# Install essential packages
RUN apt-get update && apt-get install -y \
    curl wget 
    # gnupg2 software-properties-common build-essential

#RUN apt-get update && apt-get install -y \
#    git unzip tar python3 python3-pip ca-certificates

#RUN apt-get update && apt-get install -y \
#    gnome-keyring apt-transport-https software-properties-common

# Install Node.js (LTS) and Vite
# RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && \
#    apt-get install -y nodejs && npm install -g vite

# Install Go
ENV GO_VERSION=1.22.3
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:$PATH"

# Install code-server
RUN curl -fsSL https://github.com/coder/code-server/releases/download/v4.17.1/code-server_4.17.1_amd64.deb -o /tmp/code-server.deb && \
    apt install -y /tmp/code-server.deb && \
    rm /tmp/code-server.deb

# Add the Microsoft GPG key and repository (same as above, but inside the container)
RUN wget -qO- https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor -o /usr/share/keyrings/packages.microsoft.gpg
RUN echo "deb [signed-by=/usr/share/keyrings/packages.microsoft.gpg] https://packages.microsoft.com/repos/vscode stable main" \
    | tee -a /etc/apt/sources.list.d/vscode.list


    
# Add extensions for VSCODE and Server .. These should be in project specific Dockerfiles.
USER doughznutz
ENV HOME=/home/doughznutz

RUN code-server --install-extension Continue.continue \
    && code-server --install-extension dsznajder.es7-react-js-snippets \
    && code-server --install-extension esbenp.prettier-vscode \
    && code-server --install-extension dbaeumer.vscode-eslint \
    && code-server --install-extension bradlc.vscode-tailwindcss \
    && code-server --install-extension antfu.vite \
    && code-server --install-extension golang.go

USER root

# Expose the port (if applicable; adjust as needed)
#EXPOSE 80
EXPOSE 8080

COPY config.yaml /home/doughznutz/.continue/config.yaml

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]
