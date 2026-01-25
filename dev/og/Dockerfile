# ---------- Stage 1: Build ----------
FROM golang:1.24 AS builder

WORKDIR /app
# Copy the Go source code
COPY ./*.go .

# Initialize go module if go.mod doesn't exist (useful for standalone main.go)
RUN go mod init ollama_gatewai || true 
RUN go mod tidy

# Build the Go application
# CGO_ENABLED=0 produces a statically-linked binary
# -ldflags="-s -w" strips debugging information, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -o ollama_gatewai .

# ---------- Stage 2: Runtime ----------
FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/ollama_gatewai /usr/local/bin/ollama_gatewai

ENV DOUGHZNUTZ_API_KEY=""
EXPOSE 11434

# WORKDIR is needed here for logs mount to be in the right place.
WORKDIR /app
CMD ollama_gatewai