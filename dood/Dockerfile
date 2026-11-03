FROM ubuntu:latest

# Install Core tools
RUN apt-get update \
    && apt-get install -y curl dos2unix emacs jq make \
    && apt-get clean

# Install Python
RUN apt-get update \
    && apt-get install -y python3 python3-pip python3-venv \
    && apt-get clean

# Install Node.js and npm
RUN apt-get update \
    && apt-get install -y nodejs npm \
    && apt-get clean

# Install Docker tools.
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

# Compose would otherwise name the project after /self; match the host
# checkout so containers started from either side are the same project.
ENV COMPOSE_PROJECT_NAME=d8z-cloud-env

# We will build the dev containers from here.
COPY . /self
RUN find /self -name *.sh -exec dos2unix {} +
RUN find /self -name *.sh -exec chmod +x {} +

# Setup MCP router + shared client + gateway + dashboard dependencies
COPY dood/mcp /app/mcp
RUN python3 -m venv /opt/venv_mcp
RUN /opt/venv_mcp/bin/pip install --no-cache-dir -r /app/mcp/requirements.txt

# Terminal prompt CLI (Node/inquirer)
COPY dood/cli /app/cli
RUN npm install --prefix /app/cli --no-audit --loglevel=error

# Streamlit dashboard
COPY dood/dashboard /app/dashboard

# MCP-over-HTTP gateway for external clients
COPY dood/gateway /app/gateway

# Orchestration entrypoint
COPY dood/startup.sh /app/startup.sh
RUN chmod +x /app/startup.sh

# MCP router (internal, localhost-only), MCP gateway, Streamlit dashboard
EXPOSE 3456 8766 8501

ENTRYPOINT ["/app/startup.sh"]
