"""The Docs tab: generate API docs from the workspace, and browse them."""

import os

import streamlit as st
from streamlit.components.v1 import iframe

from server import config, coverage, orchestrator
from server.extractors import REGISTRY


def render() -> None:
    """Draws the Docs tab."""
    st.caption(
        f"Extracts docs from `{config.SOURCE_DIR}` into `{config.DATA_DIR}` "
        "using go doc, typedoc, pdoc, and gopher's own readers for Python scripts "
        "and shell scripts."
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
    st.subheader("Language coverage")
    st.caption(
        "Every source file in the workspace by language, and how many sit inside "
        "something an extractor documents."
    )
    langs = coverage.scan(config.SOURCE_DIR)
    st.table(
        [
            {
                "language": c["language"],
                "extractor": c["extractor"] or "— none —",
                "files": c["files"],
                "documented": c["covered"],
            }
            for c in langs
        ]
    )
    for c in langs:
        if c["uncovered"]:
            with st.expander(f"⚠️ {c['language']}: {len(c['uncovered'])} file(s) not documented"):
                for reason, files in c["reasons"].items():
                    st.caption(reason)
                    st.code("\n".join(files))

    st.subheader("Files gopher doesn't inspect")
    st.caption(
        "Everything in the workspace that no extractor reads, so it never reaches the "
        "docs - and the report agents, who read only the docs, can't see it either."
    )
    groups = coverage.uninspected(config.SOURCE_DIR)
    if not groups:
        st.success("Every file in the workspace is inspected.")
    else:
        st.table([{"kind": g["kind"], "files": len(g["files"])} for g in groups])
        for g in groups:
            with st.expander(f"{g['kind']} ({len(g['files'])})"):
                st.code("\n".join(g["files"]))

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

    st.divider()
    st.subheader("Browse docs")
    # Each language's static doc tree is served by gopher's own in-container
    # Traefik (traefik/dynamic/base.yml), same-origin with this dashboard,
    # so it can be embedded directly - no new tab, no CORS/framing to work
    # around.
    for language in REGISTRY:
        with st.expander(f"{language} docs", expanded=False):
            iframe(f"/{config.BASE_PATH}/docs/{language}/", height=600, scrolling=True)
