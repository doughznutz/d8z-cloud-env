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
from typing import Any

import nats
from nats.js import JetStreamContext
from nats.js.errors import NotFoundError

from . import config, settings
from .models import BusReport, ChatMessage


async def _ensure_stream(js: JetStreamContext, name: str, subjects: list[str]) -> None:
    """Creates a JetStream stream if it doesn't exist yet.

    Args:
        js: An open JetStream context.
        name: Stream name.
        subjects: Subjects the stream captures.
    """
    try:
        await js.stream_info(name)
    except NotFoundError:
        await js.add_stream(name=name, subjects=subjects)


async def _fetch_stream(
    stream: str, subjects: list[str], max_msgs: int
) -> list[tuple[int, str, dict[str, Any]]]:
    """Connects to NATS and returns up to ``max_msgs`` recent messages from a stream.

    Args:
        stream: Stream name (created if missing, capturing ``subjects``).
        subjects: Subjects the stream captures.
        max_msgs: The most messages to return, oldest first.

    Returns:
        (sequence, subject, decoded JSON payload) per message, oldest first.
    """
    nc = await nats.connect(config.NATS_URL, connect_timeout=5)
    try:
        js = nc.jetstream()
        await _ensure_stream(js, stream, subjects)
        info = await js.stream_info(stream)
        out: list[tuple[int, str, dict[str, Any]]] = []
        if info.state.messages == 0:
            return out
        start = info.state.first_seq
        if info.state.last_seq - start + 1 > max_msgs:
            start = info.state.last_seq - max_msgs + 1
        for seq in range(start, info.state.last_seq + 1):
            try:
                raw = await js.get_msg(stream, seq=seq)
            except NotFoundError:
                continue  # purged/deleted sequence
            if raw.seq is None or raw.data is None:
                continue  # malformed/empty response, shouldn't happen in practice
            out.append((raw.seq, raw.subject or "", json.loads(raw.data)))
        return out
    finally:
        await nc.close()


async def fetch_chat_history(max_msgs: int) -> list[ChatMessage]:
    """Connects to NATS and returns up to ``max_msgs`` recent chat messages.

    Args:
        max_msgs: The most messages to return, oldest first.

    Returns:
        The chat history, oldest first.
    """
    msgs = await _fetch_stream(config.CHAT_STREAM, ["chat.>"], max_msgs)
    return [
        {
            "seq": seq,
            "sender": p.get("sender", "?"),
            "text": p.get("text", ""),
            "ts": p.get("ts", ""),
        }
        for seq, _, p in msgs
    ]


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


def _report_streams() -> tuple[str, list[str]]:
    """Returns the REPORTS stream's name and the subjects it captures.

    Returns:
        (stream name, subjects).
    """
    return config.REPORT_STREAM, [settings.load().report_subject + ".>"]


async def publish_report(
    run_id: str, agent: str, title: str, status: str, output: str
) -> tuple[str, int]:
    """Publishes one agent's response to ``<reports.nats_subject>.<agent>``.

    Uses a JetStream publish (not a plain one) so the returned ack proves
    the message actually landed in the REPORTS stream. Output too big for
    the server's max payload is truncated rather than failing the publish.

    Args:
        run_id: The report run it belongs to.
        agent: The agent that produced it.
        title: The run's title.
        status: The agent's final status.
        output: The agent's response.

    Returns:
        (subject, stream sequence) the message was stored at.
    """
    nc = await nats.connect(config.NATS_URL, connect_timeout=5)
    try:
        js = nc.jetstream()
        await _ensure_stream(js, *_report_streams())
        subject = f"{settings.load().report_subject}.{agent}"
        payload: dict[str, Any] = {
            "run_id": run_id,
            "agent": agent,
            "title": title,
            "status": status,
            "output": output,
            "ts": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        }
        data = json.dumps(payload, ensure_ascii=False).encode()
        # Leave headroom for headers/framing under the server's limit; a
        # char is at most 4 UTF-8 bytes, so limit // 4 chars always fits.
        limit = nc.max_payload - 4096
        if len(data) > limit:
            payload["output"] = output[: limit // 4] + "\n\n[truncated for NATS]"
            data = json.dumps(payload, ensure_ascii=False).encode()
        ack = await js.publish(subject, data, timeout=5)
        return subject, ack.seq
    finally:
        await nc.close()


async def fetch_reports(max_msgs: int) -> list[BusReport]:
    """Reads back the most recent agent responses published to the REPORTS stream.

    Args:
        max_msgs: The most messages to return, oldest first.

    Returns:
        The published reports, oldest first.
    """
    msgs = await _fetch_stream(*_report_streams(), max_msgs)
    return [
        {
            "seq": seq,
            "subject": subject,
            "run_id": p.get("run_id", ""),
            "agent": p.get("agent", "?"),
            "title": p.get("title", ""),
            "status": p.get("status", ""),
            "output": p.get("output", ""),
            "ts": p.get("ts", ""),
        }
        for seq, subject, p in msgs
    ]
