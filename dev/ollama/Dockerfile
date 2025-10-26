# ---------- Stage 1: Build ----------
FROM golang:1.24 AS builder

WORKDIR /app
# Copy the Go source code
COPY ./*.go ./

# Initialize go module if go.mod doesn't exist (useful for standalone main.go)
# If you have go.mod and go.sum, these RUN lines can be replaced with:
# COPY go.mod go.sum ./
# RUN go mod download
RUN go mod init ollama-proxy || true 
RUN go mod tidy

# Build the Go application
# CGO_ENABLED=0 produces a statically-linked binary
# -ldflags="-s -w" strips debugging information, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -o ollama-proxy .

#    go build -o ollama-proxy main.go gemini.go openai.go handlers.go postgres.go

# ---------- Stage 2: Runtime ----------
FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/ollama-proxy /usr/local/bin/ollama-proxy

ENV OPENAI_API_KEY=""
EXPOSE 11434

CMD ["ollama-proxy"]
