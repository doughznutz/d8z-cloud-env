#!/bin/bash
# Runs the Ollama server and the Streamlit dashboard as siblings under PID 1,
# forwarding signals so `docker stop` shuts both down cleanly.
set -e

ollama serve &
OLLAMA_PID=$!

echo "Waiting for Ollama API..."
until curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; do
  sleep 1
done
echo "Ollama API is ready."

streamlit run /app/app.py \
  --server.port 8501 \
  --server.address 0.0.0.0 \
  --server.headless true \
  --server.baseUrlPath og &
STREAMLIT_PID=$!

trap 'kill -TERM "$OLLAMA_PID" "$STREAMLIT_PID" 2>/dev/null' TERM INT

wait -n "$OLLAMA_PID" "$STREAMLIT_PID"
EXIT_CODE=$?
kill -TERM "$OLLAMA_PID" "$STREAMLIT_PID" 2>/dev/null || true
exit "$EXIT_CODE"
