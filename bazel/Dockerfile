# FILE: docker/env/bazel/Dockerfile

# Start from doughznutz base image
FROM env:base

# Update the apt package index and install dependencies
RUN apt-get update && apt-get install -y \
  apt-transport-https \
  curl \
  git \
  gnupg \
  lsb-release \
  software-properties-common \
  ca-certificates \
  build-essential \
  python3 \
  python3-pip \
  openjdk-11-jdk \
  && apt-get clean


# Add Bazel distribution URI as a package source
RUN curl -fsSL https://bazel.build/bazel-release.pub.gpg | gpg --dearmor > /usr/share/keyrings/bazel-archive-keyring.gpg \
  && sudo apt update && sudo apt install apt-transport-https \
  && sudo apt update

# Add Bazel apt repository to sources list
RUN echo \
  "deb [signed-by=/usr/share/keyrings/bazel-archive-keyring.gpg] https://storage.googleapis.com/bazel-apt stable jdk1.8" \
  | tee /etc/apt/sources.list.d/bazel.list

# Add the Bazel distribution URI to the apt sources
RUN apt update && apt install -y bazel

# Install Bazel (latest stable version)
RUN apt update && apt install bazel-8.0.0

# Upgrade Bazel to the latest version (optional)
# RUN apt update && apt full-upgrade -y

# Set up the Bazel environment (optional, depending on your needs)
ENV BAZEL_VERSION=8.0.0
ENV BAZEL_HOME=/usr/local/bazel

# This is to help track the Docker hierarchy, by copying the context to /docker/self
RUN [ -d "/docker/self" ] && mv /docker/self /docker/self/parent || true
COPY . /docker/self

# Pickup shortcuts from this world.
COPY ./bash_aliases bash_aliases
RUN cat bash_aliases >> ~root/.bash_aliases && rm bash_aliases

# Takes some basic tests over so we know it works in the container.
RUN git clone https://github.com/bazelbuild/examples tests/bazel

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]
