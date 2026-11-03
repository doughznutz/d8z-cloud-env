# ==============================================================================
# Stage 1: Build & Pre-cache Models
# ==============================================================================
FROM ollama/ollama:latest AS model-builder

# Start Ollama temporarily to populate the layer with core coding/reasoning models.
RUN ollama serve & \
    sleep 3 && \
    ollama pull llama3.2:3b && \
    ollama pull qwen2.5-coder:1.5b

# ==============================================================================
# Stage 2: Final Unified Runtime
# ==============================================================================
FROM ollama/ollama:latest

ENV DEBIAN_FRONTEND=noninteractive

# Ollama's base image is Ubuntu-based; add curl (health checks) and Python (dashboard).
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --break-system-packages streamlit requests

# Bring over the models pre-pulled in stage 1.
COPY --from=model-builder /root/.ollama /root/.ollama

WORKDIR /app
COPY app.py /app/app.py
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Ollama API and Streamlit dashboard.
EXPOSE 11434 8501

ENTRYPOINT ["/app/entrypoint.sh"]
