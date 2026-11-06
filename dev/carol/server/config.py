"""Shared settings for the orchestrator and extractors.

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

#: Where the orchestrator persists run history, under the data volume so it
#: survives a container restart/rebuild.
STATE_FILE: str = os.path.join(DATA_DIR, ".carol", "runs.json")

#: Oldest runs beyond this count are dropped per language, so history.json
#: doesn't grow without bound.
MAX_HISTORY: int = 25

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

#: Most chat messages to show at once.
CHAT_HISTORY: int = 50
