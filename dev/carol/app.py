"""Carol: the Streamlit dashboard.

Generates API documentation from this workspace's Go, TypeScript and
Python source by running each language's own doc-extraction tool (go doc,
typedoc, pdoc). Output goes to /data (a volume) - never back into the
source tree, which is mounted read-only.
"""

import asyncio
import os

import streamlit as st

from server import bus, config, orchestrator
from server.extractors import REGISTRY

st.set_page_config(page_title="Carol", layout="wide")
st.title("Carol — documentation generator")

docs_tab, messages_tab = st.tabs(["Docs", "Messages"])

with docs_tab:
    st.caption(
        f"Extracts docs from `{config.SOURCE_DIR}` into `{config.DATA_DIR}` "
        "using go doc, typedoc and pdoc."
    )

    # Load history once per browser session; re-reads of the same dict
    # (instead of the file) are what let repeated runs within a session
    # append in place.
    if "history" not in st.session_state:
        st.session_state.history = orchestrator.load_history()

    history = st.session_state.history

    if st.button("▶ Generate all", type="primary"):
        with st.spinner("Running extractors..."):
            for language in REGISTRY:
                orchestrator.run_language(language, history)
        st.rerun()

    st.divider()

    # One row per language: last result at a glance, plus a button to
    # re-run just that one instead of everything.
    for language in REGISTRY:
        hist = history.get(language, [])
        last = hist[-1] if hist else None
        c1, c2, c3, c4 = st.columns([2, 1, 4, 1])
        c1.markdown(f"**{language}**")
        if last:
            c2.write("🟢 ok" if last["ok"] else "🔴 failed")
            c3.write(last["detail"])
        else:
            c2.write("—")
            c3.write("not run yet")
        if c4.button("Run", key=f"run_{language}"):
            with st.spinner(f"Running {language} extractor..."):
                orchestrator.run_language(language, history)
            st.rerun()

    st.divider()
    st.subheader("History")
    picked = st.selectbox("Language", list(REGISTRY))
    rows = list(reversed(history.get(picked, [])))
    if rows:
        st.table([{k: v for k, v in r.items() if k != "targets"} for r in rows])
    else:
        st.info("No runs yet for this language.")

    st.divider()
    st.subheader("Generated files")
    # Browsing what landed under /data is the quickest way to confirm a run
    # actually wrote something, without shelling into the container.
    for language in REGISTRY:
        out_dir = os.path.join(config.DATA_DIR, language)
        if not os.path.isdir(out_dir):
            continue
        files = [
            os.path.relpath(os.path.join(root, name), config.DATA_DIR)
            for root, _, names in os.walk(out_dir)
            for name in names
        ]
        if files:
            with st.expander(f"{language} ({len(files)} file(s))"):
                st.code("\n".join(sorted(files)))

with messages_tab:
    st.caption(
        f"A chat room over dood's embedded NATS server ({config.NATS_URL}, "
        f"subject `{config.CHAT_SUBJECT}`) - see dev/dood/server/nats.go and "
        "server/bus.py. Gopher's Chat tab publishes to the same lobby, so "
        "anything sent from either dashboard shows up in both."
    )
    if "chat_name" not in st.session_state:
        st.session_state.chat_name = os.environ.get("HOSTNAME", "carol")
    st.text_input("Your name", key="chat_name")

    try:
        chat_messages = asyncio.run(bus.fetch_chat_history(config.CHAT_HISTORY))
        chat_error = None
    except Exception as e:  # surfaced to the user below, not swallowed
        chat_messages = []
        chat_error = str(e)

    if chat_error:
        st.error(f"Can't reach NATS at {config.NATS_URL}: {chat_error}")
    elif not chat_messages:
        st.info("No messages yet — say hello.")
    else:
        for m in chat_messages:
            st.markdown(f"**{m['sender']}** _{m['ts']}_  \n{m['text']}")

    with st.form("chat_send", clear_on_submit=True):
        chat_text = st.text_input("Message")
        chat_submitted = st.form_submit_button("Send")
        if chat_submitted and chat_text.strip():
            try:
                sender = st.session_state.chat_name or "carol"
                asyncio.run(bus.send_chat_message(sender, chat_text.strip()))
                st.rerun()
            except Exception as e:  # surfaced to the user below, not swallowed
                st.error(f"Send failed: {e}")

    if st.button("Refresh", key="chat_refresh"):
        st.rerun()
