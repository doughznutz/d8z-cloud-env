# Build:
#  docker build . -t d8z-cloud-container
# Help:
#  docker run -it --rm --env-file .env d8z-cloud-container help
# Run:
#  docker run -it --rm --env-file .env -v .:/work d8z-cloud-container bash
# DinD:
#  docker run -it --rm --env-file .env -v /var/run/docker.sock:/var/run/docker.sock -v .:/work d8z-cloud-container bash

# We always use ubuntu to start.
FROM ubuntu:latest

# Install necessary dependencies
RUN apt-get update
RUN apt-get install -y \
     	bash \
	curl \
	dos2unix \
	emacs \
	git \
	jq \
	openssh-client \
	coreutils

RUN apt-get install -y \
	apt-transport-https \
    	ca-certificates \
    	gnupg \
    	lsb-release


# Lets Run Docker in Docker, so we can launch the vnc.

# Add Docker's official GPG key
RUN curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

# Set up the stable Docker repository
RUN echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

# Update package lists again after adding the Docker repository
RUN apt-get update

# Install Docker CLI
RUN apt-get install -y --no-install-recommends docker-ce docker-ce-cli containerd.io

# Clean up unnecessary packages
RUN apt-get autoremove -y && apt-get clean
RUN rm -rf /var/lib/apt/lists/*

# Install docker-compose
RUN mkdir -p /usr/libexec/docker/cli-plugins
RUN curl -SL https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
         -o /usr/libexec/docker/cli-plugins/docker-compose	 
RUN chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# Copy the source code from the project directory.
COPY ./ /src
RUN rm -rf /src/.env

# Create a working directory
WORKDIR /app

# Copy scripts and ensure they are executable
COPY github/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
RUN dos2unix /app/entrypoint.sh

# Set the entrypoint
ENTRYPOINT ["/app/entrypoint.sh"]
