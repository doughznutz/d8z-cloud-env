# Use the official Ubuntu base image
FROM base:latest

# Set environment variables
ENV GO_VERSION 1.22.0 # Replace with the desired Go version
ENV GO_DOWNLOAD_URL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
ENV GO_INSTALL_DIR /usr/local

# Update package lists and install necessary dependencies
RUN apt-get update && apt-get install -y wget tar ca-certificates && rm -rf /var/lib/apt/lists/*

# Download and extract Go
RUN wget -q ${GO_DOWNLOAD_URL} && \
    tar -C ${GO_INSTALL_DIR} -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz

# Set the PATH environment variable
ENV PATH="${GO_INSTALL_DIR}/go/bin:${PATH}"

COPY test/* tests/golang/*

# Verify Bazel installation
RUN cd tests/golang && go version

# Entrypoint for the container
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]
