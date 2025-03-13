FROM ubuntu:latest

# Install necessary dependencies
RUN apt-get update
RUN apt-get install -y \
     	git \
	curl \
	dos2unix \
	jq \
	bash \
	openssh-client \
	coreutils 
RUN rm -rf /var/lib/apt/lists/*

# Create a working directory
WORKDIR /

# Copy the source code from the project directory.
COPY ./ /src
RUN rm /src/.env

# Copy scripts and ensure they are executable
COPY entrypoint.sh /src/entrypoint.sh
RUN chmod +x /src/entrypoint.sh
RUN dos2unix /src/entrypoint.sh

# Set the entrypoint
CMD ["/src/entrypoint.sh"]
