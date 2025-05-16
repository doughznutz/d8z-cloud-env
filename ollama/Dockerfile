# ---------- Stage 1: Build ----------
FROM golang:1.21 as builder

WORKDIR /app
COPY main.go .
RUN go build -o ollama-proxy main.go

# ---------- Stage 2: Runtime ----------
FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/ollama-proxy /usr/local/bin/ollama-proxy

ENV OPENAI_API_KEY=""
EXPOSE 11434

CMD ["ollama-proxy"]