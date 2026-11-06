#!/bin/sh
# Streamlit plus one static server per language's doc tree (see
# server/config.py's DATA_DIR), all on loopback only - Traefik
# (traefik/dynamic/base.yml) is the only thing actually listening on 8503.
set -e

DATA_DIR="${CAROL_DATA_DIR:-/data}"
mkdir -p "$DATA_DIR/go" "$DATA_DIR/python" "$DATA_DIR/typescript"

# $1=directory to serve, $2=port. Restarts on crash, same retry-loop style
# as dood's own entrypoint.sh backgrounds gw-server.
serve_docs() {
  (while true; do
    python3 -m http.server "$2" --directory "$1" --bind 127.0.0.1 || true
    sleep 2
  done) &
}

serve_docs "$DATA_DIR/go" 8514
serve_docs "$DATA_DIR/python" 8515
serve_docs "$DATA_DIR/typescript" 8516

(while true; do
  streamlit run /app/app.py \
    --server.port 8517 \
    --server.address 127.0.0.1 \
    --server.headless true \
    --server.baseUrlPath "${CAROL_BASE_PATH:-carol}" || true
  sleep 2
done) &

exec traefik
