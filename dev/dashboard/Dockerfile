# Stage 1: Builder
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy the Go source code
COPY main.go .

# Initialize go module if go.mod doesn't exist (useful for standalone main.go)
# If you have go.mod and go.sum, these RUN lines can be replaced with:
# COPY go.mod go.sum ./
# RUN go mod download
RUN go mod init dashboard || true 
RUN go mod tidy

# Build the Go application
# CGO_ENABLED=0 produces a statically-linked binary
# -ldflags="-s -w" strips debugging information, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -o dashboard .

# Stage 2: Runner
FROM ubuntu:latest

# Install Supervisor and any other necessary dependencies
RUN apt-get update && \
    apt-get install -y supervisor && \
    rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy the compiled application binary from the builder stage
COPY --from=builder /build/dashboard .
COPY static/index.html /app/static/index.html
# RUN chown appuser:appgroup /app/dashboard && chmod +x /app/dashboard

# Expose the ports the application listens on
EXPOSE 8080
EXPOSE 8081

# Run the dashboard
CMD ["/app/dashboard"]
