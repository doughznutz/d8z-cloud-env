# We always use ubuntu to start.
FROM ubuntu:latest

# Install necessary dependencies
RUN apt-get update \
    && apt-get install -y dos2unix \
	&& apt-get clean

RUN apt-get update \
	&& apt-get install -y curl git jq \
	&& apt-get clean

# Clean up unnecessary packages
RUN apt-get autoremove -y && apt-get clean
RUN rm -rf /var/lib/apt/lists/*

# Copy the source code from the project directory.
# This is how entrypoint "finds" the code.
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
