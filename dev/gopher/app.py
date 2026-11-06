"""Gopher: a small dashboard for defining and running checks.

Checks run against the other containers in this cluster, as plain HTTP or
raw-TCP probes from inside this container over the compose networks it
shares with its targets, so hosts/ports are the containers' own service
names and container ports - nothing needs to be published to the host.

Tests and their run history are persisted as JSON under /data (a volume),
so they survive a restart/rebuild of this container.
"""

import asyncio
import json
import os
import socket
import time
import uuid
from datetime import datetime, timezone
from typing import Any, Literal, TypedDict

import nats
import requests
import streamlit as st
from nats.js import JetStreamContext
from nats.js.errors import NotFoundError

try:
    import docker
except ImportError:  # pragma: no cover - always installed by the Dockerfile
    docker = None

DATA_DIR = os.environ.get("GOPHER_DATA_DIR", "/data")
TESTS_FILE = os.path.join(DATA_DIR, "tests.json")
RESULTS_FILE = os.path.join(DATA_DIR, "results.json")
MAX_HISTORY = 25

# dood's embedded NATS server (dev/dood/server/nats.go) opens a plain client
# listener on internal_net alongside its leafnode port - this is that one.
NATS_URL = os.environ.get("GOPHER_NATS_URL", "nats://dood:4222")
CHAT_SUBJECT = "chat.lobby"
CHAT_STREAM = "CHAT"
CHAT_HISTORY = 50

TestKind = Literal["http", "tcp", "mcp"]


class Test(TypedDict):
    """A single user-defined check against a cluster container."""

    id: str
    name: str
    kind: TestKind
    host: str
    port: int
    path: str
    method: str
    timeout_s: int
    expect_status: list[int]
    expect_contains: str
    enabled: bool


class RunOutcome(TypedDict):
    """The pass/fail verdict of a single probe, before it's timestamped."""

    ok: bool
    latency_ms: float
    detail: str


class CheckResult(RunOutcome):
    """A :class:`RunOutcome` plus the UTC time the probe ran, as stored in history."""

    ts: str


ResultsStore = dict[str, list[CheckResult]]

# Seeded on first boot only - a handful of checks against this project's own
# dev services, reachable by their compose service name on internal_net.
SEED_TESTS: list[Test] = [
    {
        "id": "dood-reachable",
        "name": "Dood gateway responds",
        "kind": "http",
        "host": "dood",
        "port": 80,
        "path": "/",
        "method": "GET",
        "timeout_s": 5,
        "expect_status": [],
        "expect_contains": "",
        "enabled": True,
    },
    {
        "id": "dozzle-reachable",
        "name": "Dozzle UI responds",
        "kind": "http",
        "host": "dozzle",
        "port": 8080,
        "path": "/",
        "method": "GET",
        "timeout_s": 5,
        "expect_status": [],
        "expect_contains": "",
        "enabled": True,
    },
    {
        "id": "deightz-reachable",
        "name": "Deightz code-server responds",
        "kind": "http",
        "host": "deightz",
        "port": 8443,
        "path": "/",
        "method": "GET",
        "timeout_s": 5,
        "expect_status": [],
        "expect_contains": "",
        "enabled": True,
    },
    {
        "id": "og-dashboard-reachable",
        "name": "Ollama gateway dashboard responds",
        "kind": "http",
        "host": "og",
        "port": 8501,
        "path": "/",
        "method": "GET",
        "timeout_s": 5,
        "expect_status": [],
        "expect_contains": "",
        "enabled": True,
    },
    {
        "id": "og-api-tags",
        "name": "Ollama API lists models",
        "kind": "http",
        "host": "og",
        "port": 11434,
        "path": "/api/tags",
        "method": "GET",
        "timeout_s": 5,
        "expect_status": [200],
        "expect_contains": "",
        "enabled": True,
    },
]


def load_json(path: str, default: Any) -> Any:
    """Load JSON from ``path``, or return ``default`` if it doesn't exist yet.

    Args:
        path: Absolute path to the JSON file.
        default: Value to return when the file is missing.

    Returns:
        The parsed JSON content, or ``default``.
    """
    if not os.path.exists(path):
        return default
    with open(path) as f:
        return json.load(f)


def save_json(path: str, data: Any) -> None:
    """Write ``data`` to ``path`` as JSON, atomically.

    Writes to a temp file in the same directory first and ``os.replace``s it
    into place, so a crash mid-write can't leave a truncated/corrupt file.

    Args:
        path: Absolute path to write to.
        data: JSON-serializable value to persist.
    """
    os.makedirs(DATA_DIR, exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2)
    os.replace(tmp, path)


def load_tests() -> list[Test]:
    """Load the saved test definitions, seeding defaults on first boot.

    Returns:
        The list of defined tests (never ``None``).
    """
    tests = load_json(TESTS_FILE, None)
    if tests is None:
        tests = [dict(t) for t in SEED_TESTS]
        save_json(TESTS_FILE, tests)
    return tests


def load_results() -> ResultsStore:
    """Load saved run history, keyed by test id.

    Returns:
        A mapping of test id to its list of past :class:`CheckResult`.
    """
    return load_json(RESULTS_FILE, {})


def run_http(test: Test) -> RunOutcome:
    """Run an HTTP check: request the test's path and judge the response.

    A test with no ``expect_status`` just needs the request to not raise
    (i.e. "reachable, whatever it says"); one with ``expect_status`` also
    requires the response code to be in that list, and one with
    ``expect_contains`` additionally requires that substring in the body.

    Args:
        test: The HTTP test to run.

    Returns:
        The outcome of the probe.
    """
    url = f"http://{test['host']}:{test['port']}{test.get('path') or '/'}"
    t0 = time.monotonic()
    try:
        resp = requests.request(test.get("method") or "GET", url, timeout=test.get("timeout_s", 5))
        latency_ms = round((time.monotonic() - t0) * 1000, 1)
        expect = test.get("expect_status") or []
        ok = (resp.status_code in expect) if expect else True
        detail = f"HTTP {resp.status_code}"
        contains = test.get("expect_contains")
        if ok and contains:
            ok = contains in resp.text
            if not ok:
                detail += " (response missing expected text)"
        return {"ok": ok, "latency_ms": latency_ms, "detail": detail}
    except requests.RequestException as e:
        return {
            "ok": False,
            "latency_ms": round((time.monotonic() - t0) * 1000, 1),
            "detail": str(e),
        }


def run_tcp(test: Test) -> RunOutcome:
    """Run a TCP check: succeed if a connection to host:port can be opened.

    Args:
        test: The TCP test to run.

    Returns:
        The outcome of the probe.
    """
    t0 = time.monotonic()
    try:
        with socket.create_connection(
            (test["host"], test["port"]), timeout=test.get("timeout_s", 5)
        ):
            pass
        return {
            "ok": True,
            "latency_ms": round((time.monotonic() - t0) * 1000, 1),
            "detail": "connected",
        }
    except OSError as e:
        return {
            "ok": False,
            "latency_ms": round((time.monotonic() - t0) * 1000, 1),
            "detail": str(e),
        }


def run_test(test: Test) -> CheckResult:
    """Dispatch ``test`` to its kind's runner and stamp the result with a time.

    Args:
        test: The test to run.

    Returns:
        The timestamped result of the probe.
    """
    if test["kind"] == "http":
        result = run_http(test)
    elif test["kind"] == "tcp":
        result = run_tcp(test)
    else:
        result = {
            "ok": False,
            "latency_ms": 0.0,
            "detail": f"{test['kind']} checks aren't wired up yet",
        }
    return {**result, "ts": datetime.now(timezone.utc).isoformat(timespec="seconds")}


def record_result(results: ResultsStore, test_id: str, result: CheckResult) -> None:
    """Append ``result`` to ``test_id``'s history, trim it, and persist it.

    Args:
        results: The full results store, updated in place.
        test_id: Id of the test this result belongs to.
        result: The result to record.
    """
    hist = results.setdefault(test_id, [])
    hist.append(result)
    del hist[:-MAX_HISTORY]
    save_json(RESULTS_FILE, results)


class ChatMessage(TypedDict):
    """One chat message, as read back from the CHAT stream."""

    seq: int
    sender: str
    text: str
    ts: str


async def _ensure_chat_stream(js: JetStreamContext) -> None:
    """Create the CHAT stream (capturing ``chat.>``) if it doesn't exist yet.

    Args:
        js: An open JetStream context.
    """
    try:
        await js.stream_info(CHAT_STREAM)
    except NotFoundError:
        await js.add_stream(name=CHAT_STREAM, subjects=["chat.>"])


async def fetch_chat_history(max_msgs: int) -> list[ChatMessage]:
    """Connect to NATS and return up to ``max_msgs`` recent chat messages.

    Args:
        max_msgs: The most messages to return, oldest first.

    Returns:
        The chat history, oldest first.
    """
    nc = await nats.connect(NATS_URL, connect_timeout=5)
    try:
        js = nc.jetstream()
        await _ensure_chat_stream(js)
        info = await js.stream_info(CHAT_STREAM)
        out: list[ChatMessage] = []
        if info.state.messages == 0:
            return out
        start = info.state.first_seq
        if info.state.last_seq - start + 1 > max_msgs:
            start = info.state.last_seq - max_msgs + 1
        for seq in range(start, info.state.last_seq + 1):
            try:
                raw = await js.get_msg(CHAT_STREAM, seq=seq)
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
    """Connect to NATS and publish one chat message.

    Args:
        sender: Display name to attribute the message to.
        text: The message body.
    """
    nc = await nats.connect(NATS_URL, connect_timeout=5)
    try:
        payload = {
            "sender": sender,
            "text": text,
            "ts": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        }
        await nc.publish(CHAT_SUBJECT, json.dumps(payload).encode())
        await nc.flush(timeout=2)
    finally:
        await nc.close()


st.set_page_config(page_title="Gopher", layout="wide")
st.title("Gopher — cluster test runner")
st.caption(
    "Enumerate the cluster's containers, define HTTP/TCP checks against them, "
    "and run/track the results."
)

if "tests" not in st.session_state:
    st.session_state.tests = load_tests()
if "results" not in st.session_state:
    st.session_state.results = load_results()

tests = st.session_state.tests
results = st.session_state.results

run_tab, define_tab, targets_tab, chat_tab = st.tabs(
    ["Run tests", "Define tests", "Targets", "Chat"]
)

with run_tab:
    if not tests:
        st.info("No tests defined yet. Add one in the **Define tests** tab.")
    else:
        if st.button("▶ Run all enabled", type="primary"):
            for t in tests:
                if t.get("enabled", True):
                    record_result(results, t["id"], run_test(t))
            st.rerun()

        st.divider()
        for t in tests:
            hist = results.get(t["id"], [])
            last = hist[-1] if hist else None
            c1, c2, c3, c4, c5 = st.columns([3, 1, 1, 3, 1])
            where = f"{t['kind']} {t['host']}:{t['port']}{t.get('path') or ''}"
            disabled_note = "" if t.get("enabled", True) else " _(disabled)_"
            c1.markdown(f"**{t['name']}**  \n`{where}`" + disabled_note)
            if last:
                c2.write("🟢 pass" if last["ok"] else "🔴 fail")
                c3.write(f"{last['latency_ms']} ms")
                c4.write(last["detail"])
            else:
                c2.write("—")
                c4.write("not run yet")
            if c5.button("Run", key=f"run_{t['id']}", disabled=not t.get("enabled", True)):
                record_result(results, t["id"], run_test(t))
                st.rerun()

        st.divider()
        st.subheader("History")
        names = [t["name"] for t in tests]
        picked = st.selectbox("Test", names)
        tid = next(t["id"] for t in tests if t["name"] == picked)
        rows = list(reversed(results.get(tid, [])))
        if rows:
            st.table(rows)
        else:
            st.info("No runs yet for this test.")

with define_tab:
    st.subheader("Add a test")
    with st.form("add_test", clear_on_submit=True):
        name = st.text_input("Name")
        kind = st.selectbox("Kind", ["http", "tcp", "mcp"])
        host = st.text_input("Host", help="The target container's service/network name, e.g. og")
        port = st.number_input("Port", min_value=1, max_value=65535, value=80)
        path = st.text_input("Path (http only)", value="/")
        method = st.selectbox("Method (http only)", ["GET", "POST", "HEAD"])
        timeout_s = st.number_input("Timeout (s)", min_value=1, max_value=60, value=5)
        expect_status = st.text_input(
            "Expected status codes, comma-separated (http only; blank = just reachable)", value=""
        )
        expect_contains = st.text_input("Response must contain (http only, optional)", value="")
        submitted = st.form_submit_button("Add test")
        if submitted:
            if not name or not host:
                st.error("Name and host are required.")
            elif kind == "mcp":
                st.error(
                    "MCP checks aren't wired up yet — there's no MCP endpoint "
                    "in the cluster to test against."
                )
            else:
                codes: list[int] | None
                try:
                    codes = [int(s) for s in expect_status.split(",") if s.strip()]
                except ValueError:
                    st.error("Expected status codes must be a comma-separated list of numbers.")
                    codes = None
                if codes is not None:
                    tests.append(
                        {
                            "id": uuid.uuid4().hex[:8],
                            "name": name,
                            "kind": kind,
                            "host": host,
                            "port": int(port),
                            "path": path,
                            "method": method,
                            "timeout_s": int(timeout_s),
                            "expect_status": codes,
                            "expect_contains": expect_contains,
                            "enabled": True,
                        }
                    )
                    save_json(TESTS_FILE, tests)
                    st.success(f"Added {name!r}.")
                    st.rerun()

    st.subheader("Existing tests")
    if not tests:
        st.info("No tests defined yet.")
    for t in list(tests):
        c1, c2, c3 = st.columns([5, 1, 1])
        where = f"{t['kind']} {t['host']}:{t['port']}{t.get('path') or ''}"
        c1.write(f"**{t['name']}** — `{where}`")
        enabled = c2.checkbox("on", value=t.get("enabled", True), key=f"en_{t['id']}")
        if enabled != t.get("enabled", True):
            t["enabled"] = enabled
            save_json(TESTS_FILE, tests)
        if c3.button("Delete", key=f"del_{t['id']}"):
            tests.remove(t)
            results.pop(t["id"], None)
            save_json(TESTS_FILE, tests)
            save_json(RESULTS_FILE, results)
            st.rerun()

with targets_tab:
    st.caption(
        "Containers visible on this host's Docker daemon (read-only). "
        "Use a container's name as the Host when defining a test against it."
    )
    if docker is None:
        st.warning("The docker SDK isn't installed in this image.")
    else:
        try:
            client = docker.from_env()
            containers = client.containers.list(all=True)
            rows = []
            for c in containers:
                ports = (c.attrs.get("NetworkSettings", {}) or {}).get("Ports", {}) or {}
                rows.append(
                    {
                        "name": c.name,
                        "image": (c.image.tags[0] if c.image.tags else c.image.short_id),
                        "status": c.status,
                        "exposed ports": ", ".join(sorted(ports.keys())) or "—",
                    }
                )
            if rows:
                st.table(rows)
            else:
                st.info("No containers visible.")
        except Exception as e:
            st.error(f"Can't reach the Docker socket: {e}")

with chat_tab:
    st.caption(
        f"A chat room over dood's embedded NATS server ({NATS_URL}, subject "
        f"`{CHAT_SUBJECT}`) - see dev/dood/server/nats.go. Anything else that "
        "publishes there shows up here too."
    )
    if "chat_name" not in st.session_state:
        st.session_state.chat_name = os.environ.get("HOSTNAME", "gopher")
    st.text_input("Your name", key="chat_name")

    try:
        chat_messages = asyncio.run(fetch_chat_history(CHAT_HISTORY))
        chat_error = None
    except Exception as e:
        chat_messages = []
        chat_error = str(e)

    if chat_error:
        st.error(f"Can't reach NATS at {NATS_URL}: {chat_error}")
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
                asyncio.run(send_chat_message(sender, chat_text.strip()))
                st.rerun()
            except Exception as e:
                st.error(f"Send failed: {e}")

    if st.button("Refresh", key="chat_refresh"):
        st.rerun()
