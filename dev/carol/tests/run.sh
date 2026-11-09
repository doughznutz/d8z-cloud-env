#!/bin/sh
# Runs carol's post-deploy test suites against running containers.
# (tests/build runs on its own, inside `docker build` - see the Dockerfile.)
#
#   tests/run.sh system          carol-agents holds nothing but the doc server
#   tests/run.sh live            cn (via og, local and free) can't stray
#   tests/run.sh all             both
#
# Defaults to the root gateway's containers; inside a workspace bundle, pass
# its names: CAROL=d8zgw-review-carol-1 AGENTS=d8zgw-review-carol-agents-1
set -e

CAROL="${CAROL:-carol}"
AGENTS="${AGENTS:-carol-agents}"

# The doc server URL exactly as carol hands it to agents (its base path
# differs per gateway), for the system suite running in carol-agents.
docs_url() {
  docker exec -w /app "$CAROL" python3 -c 'from server import config; print(config.DOCS_URL)'
}

system() {
  docker exec -w /app -e CAROL_DOCS_URL="$(docs_url)" "$AGENTS" python3 -m pytest -q tests/system
}

live() {
  docker exec -w /app "$CAROL" python3 -m pytest -q tests/live
}

case "${1:-all}" in
  system) system ;;
  live) live ;;
  all) system && live ;;
  *) echo "usage: $0 [system|live|all]" >&2; exit 2 ;;
esac
