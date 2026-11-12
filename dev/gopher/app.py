"""Gopher: the cluster's test runner and documentation generator, as one dashboard.

- Tests: define HTTP/TCP checks against the cluster's containers, run them,
  and track the results (ui/tests.py, server/checks/).
- Targets: the containers on this host's Docker daemon (ui/targets.py).
- Docs: API docs generated from this workspace's Go, TypeScript, Python
  and shell source with each language's own doc tool (ui/docs.py). Output
  goes to /data, never back into the source tree, which is mounted read-only.
- Reports: one prompt sent to several LLM agent CLIs at once, to report on
  those docs (ui/reports.py).
- Chat: the shared lobby on dood's NATS server (ui/chat.py).
"""

import streamlit as st

from server import config, settings
from ui import chat, docs, reports, targets, tests

st.set_page_config(page_title="Gopher", layout="wide")
st.title("Gopher — tests and docs")

# Settings live in the host's ~/.d8z/gopher.yaml (server/settings.py); say
# so when that isn't the file this gopher is actually using.
_settings = settings.load()
for _problem in _settings.problems:
    st.warning(f"gopher.yaml: {_problem}")
if not _settings.persistent:
    st.warning(
        f"`{config.CONFIG_FILE}` isn't mounted from the host's `~/.d8z/gopher.yaml`, so "
        "settings and presets saved here are lost when this container goes away."
    )

TABS = {"Tests": tests, "Targets": targets, "Docs": docs, "Reports": reports, "Chat": chat}
for tab, page in zip(st.tabs(list(TABS)), TABS.values()):
    with tab:
        page.render()
