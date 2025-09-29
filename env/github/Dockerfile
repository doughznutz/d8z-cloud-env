# We always use ubuntu to start.
FROM ubuntu:latest

# Install necessary dependencies
RUN apt-get update  && apt-get install -y dos2unix 	&& apt-get clean
RUN apt-get update 	&& apt-get install -y curl git jq 	&& apt-get clean

# Clean up unnecessary packages
RUN apt-get autoremove -y && apt-get clean
RUN rm -rf /var/lib/apt/lists/*

# Create a working directory
WORKDIR /self

# Copy the current directory to /self
COPY . /self

RUN find /self -name *.sh -exec dos2unix {} +
RUN find /self -name *.sh -exec chmod +x {} +

# Set the entrypoint
ENTRYPOINT ["/self/env/github/entrypoint.sh"]
