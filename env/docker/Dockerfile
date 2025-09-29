FROM ubuntu:latest

# Install Docker tools
RUN apt-get update \
    && apt-get install -y docker.io dos2unix curl jq \
    && apt-get clean

# Install docker-compose
RUN mkdir -p /usr/libexec/docker/cli-plugins
ADD https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
          /usr/libexec/docker/cli-plugins/docker-compose
RUN chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# This is to help track the Docker hierarchy
COPY . /self

RUN find /self -name *.sh -exec dos2unix {} +
RUN find /self -name *.sh -exec chmod +x {} +

# Set the entrypoint
ENTRYPOINT ["/self/env/docker/entrypoint.sh"]


