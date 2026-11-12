"""The Reports tab: one prompt sent to several LLM agent CLIs at once."""

import asyncio
import os

import streamlit as st

from server import agent_client, agents, bus, config, prompts, reports, settings
from server.extractors import REGISTRY

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


def render() -> None:
    """Draws the Reports tab."""
    st.caption(
        "Send one prompt to several LLM agent CLIs at once. They run in the separate "
        "gopher-agents container, which mounts nothing: they read the generated docs only "
        "as plain text over HTTP, and can't see the workspace source, the Docker socket, "
        "or anything else of gopher's."
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
        ok, why = avail.get(agent.name, (False, "unknown to gopher-agents"))
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
        f"Presets and the context below are stored in `~/.d8z/gopher.yaml` "
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
            "these yet - gopher just reads its own publishes, to confirm they arrived."
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
