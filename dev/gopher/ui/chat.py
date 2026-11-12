"""The Chat tab: the shared lobby on dood's NATS server."""

import asyncio
import os

import streamlit as st

from server import bus, config, settings


def render() -> None:
    """Draws the Chat tab."""
    st.caption(
        f"A chat room over dood's embedded NATS server ({config.NATS_URL}, "
        f"subject `{config.CHAT_SUBJECT}`) - see dev/dood/server/nats.go and "
        "server/bus.py. Anything else on the bus that publishes to the same "
        "subject shows up here too."
    )
    if "chat_name" not in st.session_state:
        st.session_state.chat_name = os.environ.get("HOSTNAME", "gopher")
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
                sender = st.session_state.chat_name or "gopher"
                asyncio.run(bus.send_chat_message(sender, chat_text.strip()))
                st.rerun()
            except Exception as e:  # surfaced to the user below, not swallowed
                st.error(f"Send failed: {e}")

    if st.button("Refresh", key="chat_refresh"):
        st.rerun()
