"""NATS client: a thin transport layer over dood's embedded NATS server.

Kept apart from orchestrator.py (doc extraction) and app.py (the dashboard)
on purpose: this is the one module that knows how to talk to NATS, so
whatever comes next on the bus - a job handler that runs an extractor when
asked over NATS, a presence/heartbeat publisher, anything else - only needs
to import this, not pull in Streamlit, and the extractors never need to
know NATS exists at all.
"""

import json
from datetime import datetime, timezone

import nats
from nats.js import JetStreamContext
from nats.js.errors import NotFoundError

from . import config
from .models import ChatMessage


async def _ensure_chat_stream(js: JetStreamContext) -> None:
    """Creates the shared CHAT stream (capturing ``chat.>``) if it doesn't exist yet.

    Args:
        js: An open JetStream context.
    """
    try:
        await js.stream_info(config.CHAT_STREAM)
    except NotFoundError:
        await js.add_stream(name=config.CHAT_STREAM, subjects=["chat.>"])


async def fetch_chat_history(max_msgs: int) -> list[ChatMessage]:
    """Connects to NATS and returns up to ``max_msgs`` recent chat messages.

    Args:
        max_msgs: The most messages to return, oldest first.

    Returns:
        The chat history, oldest first.
    """
    nc = await nats.connect(config.NATS_URL, connect_timeout=5)
    try:
        js = nc.jetstream()
        await _ensure_chat_stream(js)
        info = await js.stream_info(config.CHAT_STREAM)
        out: list[ChatMessage] = []
        if info.state.messages == 0:
            return out
        start = info.state.first_seq
        if info.state.last_seq - start + 1 > max_msgs:
            start = info.state.last_seq - max_msgs + 1
        for seq in range(start, info.state.last_seq + 1):
            try:
                raw = await js.get_msg(config.CHAT_STREAM, seq=seq)
            except NotFoundError:
                continue  # purged/deleted sequence
            if raw.seq is None or raw.data is None:
                continue  # malformed/empty response, shouldn't happen in practice
            payload = json.loads(raw.data)
            out.append(
                {
                    "seq": raw.seq,
                    "sender": payload.get("sender", "?"),
                    "text": payload.get("text", ""),
                    "ts": payload.get("ts", ""),
                }
            )
        return out
    finally:
        await nc.close()


async def send_chat_message(sender: str, text: str) -> None:
    """Connects to NATS and publishes one chat message to the shared lobby.

    Args:
        sender: Display name to attribute the message to.
        text: The message body.
    """
    nc = await nats.connect(config.NATS_URL, connect_timeout=5)
    try:
        payload = {
            "sender": sender,
            "text": text,
            "ts": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        }
        await nc.publish(config.CHAT_SUBJECT, json.dumps(payload).encode())
        await nc.flush(timeout=2)
    finally:
        await nc.close()
