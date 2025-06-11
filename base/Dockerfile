# This is the base Docker environtment which reflects the environtment on my laptop.
# It is intended to be used in the cloud, or on my new laptop that I havent bought yet.
FROM ubuntu:latest

# TODO: Gather nutz from docker, github, bazel, cloud

# Install Core tools
RUN apt-get update \
    && apt-get install -y make emacs \
    && apt-get clean

# Install Git
RUN apt-get update \
    && apt-get install -y git \
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

# This is to help track the Docker hierarchy, by copying the context to /docker/self
COPY . /docker/self

# Just a shell environment to get started.
CMD ["/bin/bash"]
