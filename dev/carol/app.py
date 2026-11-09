"""Carol: the Streamlit dashboard.

Generates API documentation from this workspace's Go, TypeScript and
Python source by running each language's own doc-extraction tool (go doc,
typedoc, pdoc). Output goes to /data (a volume) - never back into the
source tree, which is mounted read-only. The Reports tab then sends one
prompt to several LLM agent CLIs at once to report on those docs.
"""

import asyncio
import os

import streamlit as st
from streamlit.components.v1 import iframe

from server import (
    agent_client,
    agents,
    bus,
    config,
    coverage,
    orchestrator,
    prompts,
    reports,
    settings,
)
from server.extractors import REGISTRY

st.set_page_config(page_title="Carol", layout="wide")
st.title("Carol — documentation generator")

# Settings live in the host's ~/.d8z/carol.yaml (server/settings.py); say
# so when that isn't the file this carol is actually using.
_settings = settings.load()
for _problem in _settings.problems:
    st.warning(f"carol.yaml: {_problem}")
if not _settings.persistent:
    st.warning(
        f"`{config.CONFIG_FILE}` isn't mounted from the host's `~/.d8z/carol.yaml`, so "
        "settings and presets saved here are lost when this container goes away."
    )

docs_tab, reports_tab, messages_tab = st.tabs(["Docs", "Reports", "Messages"])

with docs_tab:
    st.caption(
        f"Extracts docs from `{config.SOURCE_DIR}` into `{config.DATA_DIR}` "
        "using go doc, typedoc, pdoc, and carol's own readers for Python scripts "
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
            why = "no extractor for this language" if not c["extractor"] else "outside any project"
            with st.expander(f"⚠️ {c['language']}: {len(c['uncovered'])} file(s) not documented"):
                st.caption(why)
                st.code("\n".join(c["uncovered"]))

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
    # Each language's static doc tree is served by carol's own in-container
    # Traefik (traefik/dynamic/base.yml), same-origin with this dashboard,
    # so it can be embedded directly - no new tab, no CORS/framing to work
    # around.
    for language in REGISTRY:
        with st.expander(f"{language} docs", expanded=False):
            iframe(f"/{config.BASE_PATH}/docs/{language}/", height=600, scrolling=True)

STATUS_ICON = {
    "running": "⏳",
    "ok": "🟢",
    "failed": "🔴",
    "timeout": "⏱️",
    "interrupted": "⚪",
}


def _load_preset() -> None:
    """Copies the picked preset into the editable title/prompt fields."""
    picked = next((p for p in prompts.all_prompts() if p["title"] == st.session_state.preset), None)
    if picked:
        st.session_state.prompt_title = picked["title"]
        st.session_state.prompt_text = picked["text"]


def _show_run(run_id: str) -> None:
    """Renders one report run's per-agent responses.

    Re-runs itself every few seconds (as a fragment, so only this part of
    the page refreshes) for as long as any agent is still going.

    Args:
        run_id: The run to show.
    """

    def body() -> None:
        if still_running and not reports.is_running(run_id):
            st.rerun()  # finished: full rerun, which stops the polling below
        run = reports.load(run_id)
        if run is None:
            st.warning(f"Run {run_id} not found.")
            return
        st.markdown(f"**{run['title']}** — started {run['ts']}")
        with st.expander("Full prompt sent"):
            st.code(run["prompt"], language="markdown", wrap_lines=True)
        names = list(run["results"])
        tabs = st.tabs(
            [
                (
                    f"{STATUS_ICON.get(run['results'][n]['status'], '?')} {agents.AGENTS[n].label}"
                    if n in agents.AGENTS
                    else n
                )
                for n in names
            ]
        )
        for tab, n in zip(tabs, names):
            r = run["results"][n]
            with tab:
                if r["status"] == "running":
                    st.info(f"Running since {r['started']}…")
                    continue
                meta = f"{r['status']} · {r['duration_s']}s · exit {r['exit_code']}"
                if r["published"] is True:
                    meta += f" · published to NATS `{r['publish_detail']}`"
                st.caption(meta)
                if r["published"] is False:
                    st.error(f"NATS publish failed: {r['publish_detail']}")
                if r["output"]:
                    st.markdown(r["output"])
                else:
                    st.warning("No output on stdout.")
                if r["stderr"]:
                    with st.expander("stderr"):
                        st.code(r["stderr"], wrap_lines=True)

    still_running = reports.is_running(run_id)
    st.fragment(body, run_every=3 if still_running else None)()


with reports_tab:
    st.caption(
        "Send one prompt to several LLM agent CLIs at once. They run in the separate "
        "carol-agents container, whose only mount is the generated docs (read-only) - "
        "they can't see the workspace source or anything else of carol's."
    )

    if not any(
        files
        for language in REGISTRY
        for _, _, files in os.walk(os.path.join(config.DATA_DIR, language))
    ):
        st.warning("No docs generated yet — run **Generate all** on the Docs tab first.")

    st.subheader("Agents")
    selected: list[str] = []
    agent_cols = st.columns(len(agents.AGENTS))
    avail = agent_client.availability()
    for col, agent in zip(agent_cols, agents.AGENTS.values()):
        ok, why = avail.get(agent.name, (False, "unknown to carol-agents"))
        with col:
            if st.checkbox(agent.label, value=ok, disabled=not ok, key=f"agent_{agent.name}"):
                selected.append(agent.name)
            st.caption(("✅ " if ok else "⚠️ ") + why)

    st.subheader("Prompt")
    all_presets = prompts.all_prompts()
    if "prompt_text" not in st.session_state and all_presets:
        st.session_state.preset = all_presets[0]["title"]
        _load_preset()
    st.selectbox(
        "Preset",
        [p["title"] for p in all_presets],
        key="preset",
        on_change=_load_preset,
    )
    st.text_input("Title", key="prompt_title")
    st.text_area("Prompt", key="prompt_text", height=150)
    st.caption(
        f"Presets and the context below are stored in `~/.d8z/carol.yaml` "
        f"(`{config.CONFIG_FILE}` here) - edit it by hand or save from here."
    )
    with st.expander("Context prepended to every prompt"):
        st.code(prompts.with_context("<your prompt>"), language="markdown", wrap_lines=True)

    p1, p2, _ = st.columns([1, 1, 4])
    title = (st.session_state.get("prompt_title") or "").strip()
    text = (st.session_state.get("prompt_text") or "").strip()
    if p1.button("💾 Save preset", disabled=not (title and text)):
        try:
            prompts.save(title, text)
            st.toast(f"Saved preset “{title}”")
            st.rerun()
        except (OSError, ValueError) as e:  # shown, never a silent overwrite
            st.error(f"Couldn't save to {config.CONFIG_FILE}: {e}")
    if any(p["title"] == title for p in all_presets) and p2.button("🗑 Delete preset"):
        try:
            prompts.delete(title)
            st.toast(f"Deleted preset “{title}”")
            st.rerun()
        except (OSError, ValueError) as e:  # shown, never a silent overwrite
            st.error(f"Couldn't save to {config.CONFIG_FILE}: {e}")

    publish = st.checkbox(
        f"Also publish each response to NATS (`{settings.load().report_subject}.<agent>`)",
        value=False,
    )
    if st.button(
        f"▶ Run on {len(selected)} agent(s)",
        type="primary",
        disabled=not (selected and text),
    ):
        st.session_state.report_run = reports.start(
            title or "untitled", prompts.with_context(text), selected, publish
        )
        st.rerun()

    st.divider()
    st.subheader("Responses")
    run_ids = reports.list_ids()
    if not run_ids:
        st.info("No report runs yet.")
    else:
        current = st.session_state.get("report_run")
        default = run_ids.index(current) if current in run_ids else 0
        run_id = st.selectbox("Run", run_ids, index=default)
        st.session_state.report_run = run_id
        _show_run(run_id)

    st.divider()
    with st.expander(f"Reports on the NATS bus (stream `{config.REPORT_STREAM}`)"):
        st.caption(
            f"Read back from `{settings.load().report_subject}.>`. Nothing else consumes "
            "these yet - carol just reads its own publishes, to confirm they arrived."
        )
        if st.button("Load", key="bus_reports_load"):
            try:
                bus_reports = asyncio.run(bus.fetch_reports(settings.load().bus_history))
                if not bus_reports:
                    st.info("Nothing published yet.")
                for br in reversed(bus_reports):
                    st.markdown(
                        f"**#{br['seq']}** `{br['subject']}` — {br['title']} "
                        f"({br['status']}, run {br['run_id']}, {br['ts']})"
                    )
                    st.text(br["output"][:500] + ("…" if len(br["output"]) > 500 else ""))
            except Exception as e:  # surfaced to the user, not swallowed
                st.error(f"Can't reach NATS at {config.NATS_URL}: {e}")

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
        chat_messages = asyncio.run(bus.fetch_chat_history(settings.load().chat_history))
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
