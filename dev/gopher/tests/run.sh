#!/bin/sh
# Runs gopher's post-deploy test suites against running containers.
# (tests/build runs on its own, inside `docker build` - see the Dockerfile.)
#
#   tests/run.sh system          gopher-agents holds nothing but the doc server
#   tests/run.sh workspace       every extractor works on the real workspace
#   tests/run.sh live            cn (via og, local and free) can't stray
#   tests/run.sh all             all three
#
# Defaults to the root gateway's containers; inside a workspace bundle, pass
# its names: GOPHER=d8zgw-review-gopher-1 AGENTS=d8zgw-review-gopher-agents-1
set -e

GOPHER="${GOPHER:-gopher}"
AGENTS="${AGENTS:-gopher-agents}"

# The doc server URL exactly as gopher hands it to agents (its base path
# differs per gateway), for the system suite running in gopher-agents.
docs_url() {
  docker exec -w /app "$GOPHER" python3 -c 'from server import config; print(config.DOCS_URL)'
}

system() {
  docker exec -w /app -e GOPHER_DOCS_URL="$(docs_url)" "$AGENTS" python3 -m pytest -q tests/system
}

workspace() {
  docker exec -w /app "$GOPHER" python3 -m pytest -q -p no:cacheprovider tests/workspace
}

live() {
  docker exec -w /app "$GOPHER" python3 -m pytest -q tests/live
}

case "${1:-all}" in
  system) system ;;
  workspace) workspace ;;
  live) live ;;
  all) system && workspace && live ;;
  *) echo "usage: $0 [system|workspace|live|all]" >&2; exit 2 ;;
esac
