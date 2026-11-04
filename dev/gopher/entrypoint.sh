#!/bin/sh
set -e

exec streamlit run /app/app.py \
  --server.port 8502 \
  --server.address 0.0.0.0 \
  --server.headless true \
  --server.baseUrlPath "${GOPHER_BASE_PATH:-gopher}"
