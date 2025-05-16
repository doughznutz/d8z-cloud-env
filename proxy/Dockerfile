# Stage 1: Builder
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy the Go source code
COPY main.go .

# Initialize go module if go.mod doesn't exist (useful for standalone main.go)
# If you have go.mod and go.sum, these RUN lines can be replaced with:
# COPY go.mod go.sum ./
# RUN go mod download
RUN go mod init proxy || true 
RUN go mod tidy

# Build the Go application
# CGO_ENABLED=0 produces a statically-linked binary
# -ldflags="-s -w" strips debugging information, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -o proxyapp .

# Stage 2: Runner
FROM ubuntu:latest

# Install Supervisor and any other necessary dependencies
RUN apt-get update && \
    apt-get install -y supervisor && \
    rm -rf /var/lib/apt/lists/*

# Create a directory for Supervisor logs
RUN mkdir -p /var/log/supervisor

# Create a non-root user to run the application
# RUN groupadd -r appgroup &amp;&amp; useradd -r -g appgroup -s /sbin/nologin -c "Application User" appuser

# Set working directory
WORKDIR /app

# Copy the compiled application binary from the builder stage
COPY --from=builder /app/proxyapp .
COPY static/index.html /app/static/index.html
# RUN chown appuser:appgroup /app/proxyapp && chmod +x /app/proxyapp

# Copy the Supervisor configuration file
COPY proxy-supervisor.conf /etc/supervisor/conf.d/proxy-supervisor.conf

# Expose the ports the application listens on
EXPOSE 8080
EXPOSE 8081

# Command to run Supervisor
#CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf"]
CMD ["/app/proxyapp"]
