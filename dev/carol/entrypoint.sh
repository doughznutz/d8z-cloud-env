#!/bin/sh
set -e

exec streamlit run /app/app.py \
  --server.port 8503 \
  --server.address 0.0.0.0 \
  --server.headless true \
  --server.baseUrlPath "${CAROL_BASE_PATH:-carol}"
