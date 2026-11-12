"""The Targets tab: the containers on this host's Docker daemon."""

import streamlit as st

from server import targets


def render() -> None:
    """Draws the Targets tab."""
    st.caption(
        "Containers on this host's Docker daemon. Use a container's service name as "
        "the Host when defining a check against it."
    )
    try:
        rows = targets.list_targets()
    except Exception as e:  # surfaced to the user, not swallowed
        st.error(f"Can't reach the Docker socket: {e}")
        return
    if rows:
        st.table(rows)
    else:
        st.info("No containers visible.")
    if st.button("Refresh", key="targets_refresh"):
        st.rerun()
