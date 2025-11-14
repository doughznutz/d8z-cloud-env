FROM dev:vnc
ARG USERNAME
ARG IDE_TYPE=vscode

# Install essential packages
RUN apt-get update && apt-get install -y \
    curl wget gnupg2 software-properties-common build-essential \
    git unzip tar python3 python3-pip ca-certificates \
    gnome-keyring apt-transport-https

# Install Node.js (LTS) and Vite
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && \
    apt-get install -y nodejs && npm install -g vite

# Install Go
ENV GO_VERSION=1.22.3
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:$PATH"

# Install VS Code or code-server based on IDE_TYPE
RUN if [ "$IDE_TYPE" = "vscode" ]; then \
        curl https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor > microsoft.gpg && \
        install -o root -g root -m 644 microsoft.gpg /etc/apt/trusted.gpg.d/ && \
        sh -c 'echo "deb [arch=amd64] https://packages.microsoft.com/repos/vscode stable main" > /etc/apt/sources.list.d/vscode.list' && \
        apt-get update && apt-get install -y code && \
        rm microsoft.gpg && export IDE_BIN="code"; \
    elif [ "$IDE_TYPE" = "codeserver" ]; then \
        curl -fsSL https://github.com/coder/code-server/releases/download/v4.21.2/code-server_4.21.2_amd64.deb -o /tmp/code-server.deb && \
        apt install -y /tmp/code-server.deb && \
        rm /tmp/code-server.deb && export IDE_BIN="code-server"; \
    fi

# Tell vscode to not worry about WSL.
ENV DONT_PROMPT_WSL_INSTALL=IS_THIS_NECESSARY

# Add extensions as the user.
USER $USERNAME
ENV HOME=/home/$USERNAME

RUN if [ "$IDE_TYPE" = "vscode" ]; then \
        export IDE_BIN="code"; \
    elif [ "$IDE_TYPE" = "codeserver" ]; then \
        export IDE_BIN="code-server"; \
    fi \
    && $IDE_BIN --install-extension Continue.continue \
    && $IDE_BIN --install-extension golang.go \
    && $IDE_BIN --install-extension ms-python.python

# Expose ports
EXPOSE 80
EXPOSE 8080

# The Continue.continue setup
COPY ./continue/ /home/$USERNAME/.continue/

# Entrypoint for the container
USER root
COPY ./codeserver.conf /tmp/codeserver.conf
RUN if [ "$IDE_TYPE" = "codeserver" ]; then \
 cp /tmp/codeserver.conf /etc/supervisor/conf.d/codeserver.conf; \
fi

