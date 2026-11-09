"""Per-instance wiring, from the environment compose / gw-server set.

These differ between carols (each workspace's has its own base path and
data volume), so they can't live in the shared settings file. Everything
a person would tune instead lives in ~/.d8z/carol.yaml - see settings.py.

CAROL_SOURCE_DIR is the workspace mounted read-only; CAROL_DATA_DIR is the
volume generated docs are written to - they must never be the same path.
"""

import os

#: Read-only mount of the workspace to extract docs from.
SOURCE_DIR: str = os.environ.get("CAROL_SOURCE_DIR", "/source")

#: Writable volume that generated docs (and run history) are stored under.
DATA_DIR: str = os.environ.get("CAROL_DATA_DIR", "/data")

#: This dashboard's own mount point behind carol's in-container Traefik
#: (traefik/dynamic/base.yml) - the same env var entrypoint.sh already
#: passes Streamlit as --server.baseUrlPath, needed here too so app.py can
#: build iframe URLs for the per-language doc trees that actually resolve.
BASE_PATH: str = os.environ.get("CAROL_BASE_PATH", "carol")

#: Carol's own state (doc-run history, report runs) - its own volume, kept
#: apart from DATA_DIR because everything under DATA_DIR is served, and the
#: report agents read what's served (not, say, other agents' past reports).
STATE_DIR: str = os.environ.get("CAROL_STATE_DIR", "/state")

#: Where the orchestrator persists run history.
STATE_FILE: str = os.path.join(STATE_DIR, "runs.json")

#: dood's embedded NATS server (dev/dood/server/nats.go) opens a plain
#: client listener on internal_net alongside its leafnode port - this is
#: that one.
NATS_URL: str = os.environ.get("CAROL_NATS_URL", "nats://dood:4222")

#: The shared lobby subject gopher's Chat tab also publishes to - dood's
#: NATS server has no concept of "rooms" beyond the subject name, so
#: anything sent from either dashboard shows up in both.
CHAT_SUBJECT: str = "chat.lobby"

#: JetStream stream capturing every "chat.>" subject, so the lobby's
#: history survives a dashboard restart.
CHAT_STREAM: str = "CHAT"

#: Where each LLM report run (prompt + every agent's response) is persisted,
#: one JSON file per run.
REPORTS_DIR: str = os.path.join(STATE_DIR, "reports")

#: JetStream stream capturing every "<prefix>.>" subject, so published
#: reports can be read back after the fact.
REPORT_STREAM: str = "REPORTS"

#: The host's ~/.d8z/carol.yaml, bind mounted on its own (the rest of
#: ~/.d8z holds secrets) - see settings.py.
CONFIG_FILE: str = os.environ.get("CAROL_CONFIG_FILE", "/etc/d8z/carol.yaml")

#: The carol-agents service (server/agent_runner.py), which runs the agent
#: CLIs where they can see only the docs.
AGENTS_URL: str = os.environ.get("CAROL_AGENTS_URL", "http://carol-agents:8519")

#: The doc trees as the report agents reach them: carol's own Traefik, from
#: carol-agents, over the network ("carol" resolves at the root gateway and
#: inside every bundle). Agents read pages only as ``<url>?format=text``
#: (server/docserver.py) - they have no file access to the docs at all.
DOCS_URL: str = os.environ.get("CAROL_DOCS_URL", f"http://carol:8503/{BASE_PATH}/docs/")
