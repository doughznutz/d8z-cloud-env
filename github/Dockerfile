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
RUN apt-get update && \
    apt-get install -y \
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



# Clean up unnecessary packages
RUN apt-get autoremove -y && apt-get clean
RUN rm -rf /var/lib/apt/lists/*

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
