#!/bin/sh
# Streamlit plus one static server per language's doc tree (see
# server/config.py's DATA_DIR), all on loopback only - Traefik
# (traefik/dynamic/base.yml) is the only thing actually listening on 8502.
set -e

DATA_DIR="${GOPHER_DATA_DIR:-/data}"
STATE_DIR="${GOPHER_STATE_DIR:-/state}"
mkdir -p "$DATA_DIR/go" "$DATA_DIR/python" "$DATA_DIR/typescript" "$DATA_DIR/shell" "$STATE_DIR"

# One-time migration from carol, which this gopher absorbed: carol's doc-run
# history and report runs, from its old state volume (mounted read-only at
# $LEGACY_CAROL by docker-compose.yaml). Gopher's own checks (tests.json,
# results.json) need no moving - $STATE_DIR is gopher's old data volume.
# Never overwrites: skipped once runs.json or reports/ exist here.
LEGACY_CAROL="${GOPHER_LEGACY_CAROL_DIR:-/legacy/carol}"
if [ -d "$LEGACY_CAROL" ] && [ ! -e "$STATE_DIR/runs.json" ] && [ ! -e "$STATE_DIR/reports" ]; then
  for f in runs.json reports prompts.json; do
    if [ -e "$LEGACY_CAROL/$f" ]; then cp -a "$LEGACY_CAROL/$f" "$STATE_DIR/"; fi
  done
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
    --server.baseUrlPath "${GOPHER_BASE_PATH:-gopher}" || true
  sleep 2
done) &

exec traefik
