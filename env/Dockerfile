FROM ubuntu:latest

# Install Core tools
RUN apt-get update \
    && apt-get install -y curl dos2unix emacs jq make \
    && apt-get clean

# Install Git - Do we want GH?
RUN apt-get update \
    && apt-get install -y git \
    && apt-get clean

# Install Python
RUN apt-get update \
    && apt-get install -y python3 python3-pip python3-venv \
    && apt-get clean

# Install Node.js and npm
RUN apt-get update \
    && apt-get install -y nodejs npm \
    && apt-get clean

# Install Docker tools
RUN apt-get update \
    && apt-get install -y docker.io \
    && apt-get clean

# Install docker-compose
RUN mkdir -p /usr/libexec/docker/cli-plugins
ADD https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
          /usr/libexec/docker/cli-plugins/docker-compose
RUN chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# Clean up unnecessary packages
RUN apt-get autoremove -y && apt-get clean
RUN rm -rf /var/lib/apt/lists/*

# Create a working directory
WORKDIR /self

# We will build the dev containers from here.
COPY . /self
RUN find /self -name *.sh -exec dos2unix {} +
RUN find /self -name *.sh -exec chmod +x {} +

# Setup MCP server
COPY env/mcp /app/mcp
RUN python3 -m venv /opt/venv_mcp
RUN /opt/venv_mcp/bin/pip install --no-cache-dir -r /app/mcp/requirements.txt

# Setup Node.js Entrypoint Client
COPY env/entrypoint /app/entrypoint
RUN npm install --prefix /app/entrypoint --no-audit --loglevel=error

# Set the entrypoint
ENTRYPOINT ["node", "/app/entrypoint/index.js"]
