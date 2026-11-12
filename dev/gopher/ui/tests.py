"""The Tests tab: define HTTP/TCP checks against cluster containers, run and track them."""

import streamlit as st

from server.checks import REGISTRY, runner
from server.models import Check, CheckHistory


def _where(c: Check) -> str:
    """One-line description of what a check probes.

    Args:
        c: The check.

    Returns:
        e.g. ``http og:11434/api/tags``.
    """
    path = (c.get("path") or "") if c["kind"] == "http" else ""
    return f"{c['kind']} {c['host']}:{c['port']}{path}"


def _run_section(checks: list[Check], results: CheckHistory) -> None:
    """Draws every check's latest result, run buttons, and a per-check history.

    Args:
        checks: The defined checks.
        results: Their run history, updated in place by any run.
    """
    if not checks:
        st.info("No checks defined yet. Add one below.")
        return
    if st.button("▶ Run all enabled", type="primary"):
        with st.spinner("Running checks..."):
            runner.run_enabled(checks, results)
        st.rerun()

    st.divider()
    for c in checks:
        hist = results.get(c["id"], [])
        last = hist[-1] if hist else None
        enabled = c.get("enabled", True)
        c1, c2, c3, c4, c5 = st.columns([3, 1, 1, 3, 1])
        c1.markdown(f"**{c['name']}**  \n`{_where(c)}`" + ("" if enabled else " _(disabled)_"))
        if last:
            c2.write("🟢 pass" if last["ok"] else "🔴 fail")
            c3.write(f"{last['latency_ms']} ms")
            c4.write(last["detail"])
        else:
            c2.write("—")
            c4.write("not run yet")
        if c5.button("Run", key=f"run_check_{c['id']}", disabled=not enabled):
            runner.run_and_record(c, results)
            st.rerun()

    st.divider()
    st.subheader("History")
    names = {c["id"]: c["name"] for c in checks}
    picked = st.selectbox("Check", list(names), format_func=names.__getitem__)
    rows = list(reversed(results.get(picked, []))) if picked else []
    if rows:
        st.table(rows)
    else:
        st.info("No runs yet for this check.")


def _define_section(checks: list[Check], results: CheckHistory) -> None:
    """Draws the add-a-check form and the enable/delete list.

    Args:
        checks: The defined checks, updated in place.
        results: Their run history (a deleted check's goes with it).
    """
    st.subheader("Add a check")
    with st.form("add_check", clear_on_submit=True):
        name = st.text_input("Name")
        kind = st.selectbox("Kind", list(REGISTRY))
        host = st.text_input("Host", help="The target container's service/network name, e.g. og")
        port = st.number_input("Port", min_value=1, max_value=65535, value=80)
        path = st.text_input("Path (http only)", value="/")
        method = st.selectbox("Method (http only)", ["GET", "POST", "HEAD"])
        timeout_s = st.number_input("Timeout (s)", min_value=1, max_value=60, value=5)
        expect_status = st.text_input(
            "Expected status codes, comma-separated (http only; blank = just reachable)"
        )
        expect_contains = st.text_input("Response must contain (http only, optional)")
        if st.form_submit_button("Add check"):
            try:
                check = runner.new_check(
                    name,
                    str(kind),
                    host,
                    int(port),
                    path,
                    str(method),
                    int(timeout_s),
                    expect_status,
                    expect_contains,
                )
            except ValueError as e:
                st.error(str(e))
            else:
                checks.append(check)
                runner.save_checks(checks)
                st.toast(f"Added “{check['name']}”")
                st.rerun()

    st.subheader("Defined checks")
    if not checks:
        st.info("No checks defined yet.")
    for c in list(checks):
        c1, c2, c3 = st.columns([5, 1, 1])
        c1.write(f"**{c['name']}** — `{_where(c)}`")
        enabled = c2.checkbox("on", value=c.get("enabled", True), key=f"en_{c['id']}")
        if enabled != c.get("enabled", True):
            c["enabled"] = enabled
            runner.save_checks(checks)
        if c3.button("Delete", key=f"del_{c['id']}"):
            runner.delete(checks, results, c["id"])
            st.rerun()


def render() -> None:
    """Draws the Tests tab."""
    st.caption(
        "HTTP/TCP checks against the cluster's containers, probed from inside gopher over "
        "the networks it shares with them - so use service names and container ports; "
        "nothing needs publishing to the host. See the **Targets** tab for what's running."
    )
    # Loaded once per browser session, then updated in place, so a run's
    # result shows without re-reading the files.
    if "checks" not in st.session_state:
        st.session_state.checks = runner.load_checks()
    if "check_results" not in st.session_state:
        st.session_state.check_results = runner.load_results()
    checks: list[Check] = st.session_state.checks
    results: CheckHistory = st.session_state.check_results

    _run_section(checks, results)
    st.divider()
    _define_section(checks, results)
