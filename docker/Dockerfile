# FILE: env/docker/Dockerfile

# Use doughznutz base ubuntu image
FROM env:base

# Install Docker tools
RUN apt-get update && apt-get install -y \
    docker.io \
    && apt-get clean


# Install docker-compose
RUN mkdir -p /usr/libexec/docker/cli-plugins
ADD https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
          /usr/libexec/docker/cli-plugins/docker-compose
RUN chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# Set up Docker group (optional: allows non-root Docker access)
# RUN groupadd docker && usermod -aG docker root

# This is to help track the Docker hierarchy, by copying the context to /docker/self
RUN [ -d "/docker/self" ] && mv /docker/self /docker/self/parent || true
COPY . /docker/self

# Update bash aliases.
COPY ./bash_aliases bash_aliases
RUN cat bash_aliases >> ~root/.bash_aliases && rm bash_aliases

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]