#!/bin/sh
# Streamlit plus one static server per language's doc tree (see
# server/config.py's DATA_DIR), all on loopback only - Traefik
# (traefik/dynamic/base.yml) is the only thing actually listening on 8503.
set -e

DATA_DIR="${CAROL_DATA_DIR:-/data}"
STATE_DIR="${CAROL_STATE_DIR:-/state}"
mkdir -p "$DATA_DIR/go" "$DATA_DIR/python" "$DATA_DIR/typescript" "$DATA_DIR/shell" "$STATE_DIR"

# Carol's state (run history, report runs) used to live in $DATA_DIR/.carol.
# It has its own volume, kept apart from the published docs: everything
# under $DATA_DIR is served, and the report agents read what's served.
if [ -d "$DATA_DIR/.carol" ] && [ -z "$(ls -A "$STATE_DIR")" ]; then
  cp -a "$DATA_DIR/.carol/." "$STATE_DIR/"
  rm -rf "$DATA_DIR/.carol"
fi

# $1=directory to serve, $2=port. Restarts on crash, same retry-loop style
# as dood's own entrypoint.sh backgrounds gw-server. server/docserver.py is
# a static server that also renders any page as plain text (?format=text) -
# the view the report agents read.
serve_docs() {
  (while true; do
    PYTHONPATH=/app python3 -m server.docserver "$2" "$1" || true
    sleep 2
  done) &
}

serve_docs "$DATA_DIR/go" 8514
serve_docs "$DATA_DIR/python" 8515
serve_docs "$DATA_DIR/typescript" 8516
serve_docs "$DATA_DIR/shell" 8518

(while true; do
  streamlit run /app/app.py \
    --server.port 8517 \
    --server.address 127.0.0.1 \
    --server.headless true \
    --server.baseUrlPath "${CAROL_BASE_PATH:-carol}" || true
  sleep 2
done) &

exec traefik
