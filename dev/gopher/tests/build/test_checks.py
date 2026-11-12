"""Checks validate their input, probe for real, and keep a bounded history.

Runs against throwaway local listeners on loopback - no cluster needed - with
check state redirected to a temp dir, never the image's STATE_DIR.
"""

import http.server
import socket
import threading
from collections.abc import Iterator
from pathlib import Path

import pytest

from server import config, settings
from server.checks import REGISTRY, runner
from server.models import Check

#: The real check paths, captured before the autouse fixture redirects them.
REAL_PATHS = (config.CHECKS_FILE, config.RESULTS_FILE)


@pytest.fixture(autouse=True)
def state(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    """Points the check files at a temp dir and keeps 3 results per check.

    Args:
        tmp_path: pytest's per-test temporary directory.
        monkeypatch: pytest's monkeypatch fixture.

    Returns:
        The temp state dir.
    """
    monkeypatch.setattr(config, "CHECKS_FILE", str(tmp_path / "tests.json"))
    monkeypatch.setattr(config, "RESULTS_FILE", str(tmp_path / "results.json"))
    monkeypatch.setattr(settings, "load", lambda: settings.Settings(checks_keep_runs=3))
    return tmp_path


class _Handler(http.server.BaseHTTPRequestHandler):
    """Answers every GET with 200 "hello gopher", and /missing with 404."""

    def do_GET(self) -> None:  # noqa: N802 - the stdlib's naming
        """Serves the canned response."""
        code = 404 if self.path == "/missing" else 200
        body = b"hello gopher"
        self.send_response(code)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: object) -> None:  # noqa: A002
        """Keeps test output quiet."""


@pytest.fixture
def http_port() -> Iterator[int]:
    """Runs a tiny HTTP server on loopback for the duration of a test.

    Yields:
        Its port.
    """
    server = http.server.HTTPServer(("127.0.0.1", 0), _Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        yield server.server_address[1]
    finally:
        server.shutdown()
        server.server_close()


def _closed_port() -> int:
    """Finds a loopback port nothing is listening on.

    Returns:
        The port.
    """
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


def _check(kind: str, port: int, path: str = "/", status: str = "", contains: str = "") -> Check:
    """Builds a check against loopback.

    Args:
        kind: Check kind.
        port: Target port.
        path: HTTP path.
        status: Expected status codes, comma-separated.
        contains: Required response text.

    Returns:
        The check.
    """
    return runner.new_check(
        "t",
        kind,
        "127.0.0.1",
        port,
        path,
        timeout_s=2,
        expect_status=status,
        expect_contains=contains,
    )


def test_every_kind_has_a_prober() -> None:
    """The kinds the UI offers are exactly the ones that can run."""
    assert set(REGISTRY) == {"http", "tcp"}


@pytest.mark.parametrize(
    "kwargs, message",
    [
        ({"name": " ", "host": "og"}, "required"),
        ({"name": "x", "host": "og", "kind": "mcp"}, "Unknown check kind"),
        ({"name": "x", "host": "og", "port": 0}, "Port"),
        ({"name": "x", "host": "og", "expect_status": "200,abc"}, "comma-separated"),
    ],
)
def test_new_check_rejects_bad_input(kwargs: dict[str, object], message: str) -> None:
    """Invalid form input is refused with a message saying what's wrong.

    Args:
        kwargs: Overrides for a valid set of fields.
        message: Text the error must contain.
    """
    fields: dict[str, object] = {"name": "x", "kind": "http", "host": "og", "port": 80}
    fields.update(kwargs)
    with pytest.raises(ValueError, match=message):
        runner.new_check(**fields)  # type: ignore[arg-type]


def test_http_pass_fail_and_contains(http_port: int) -> None:
    """HTTP checks judge reachability, status codes and body text.

    Args:
        http_port: The local HTTP server's port.
    """
    assert runner.run_check(_check("http", http_port))["ok"]
    assert runner.run_check(_check("http", http_port, "/missing"))["ok"]  # just reachable
    assert not runner.run_check(_check("http", http_port, "/missing", "200"))["ok"]
    assert runner.run_check(_check("http", http_port, contains="gopher"))["ok"]
    miss = runner.run_check(_check("http", http_port, contains="carol"))
    assert not miss["ok"] and "missing expected text" in miss["detail"]
    assert not runner.run_check(_check("http", _closed_port()))["ok"]


def test_tcp_pass_and_fail(http_port: int) -> None:
    """TCP checks pass on an open port and fail on a closed one.

    Args:
        http_port: An open port.
    """
    assert runner.run_check(_check("tcp", http_port))["detail"] == "connected"
    assert not runner.run_check(_check("tcp", _closed_port()))["ok"]


def test_history_is_trimmed_and_persisted(http_port: int, state: Path) -> None:
    """Each check keeps only checks.keep_runs results, and they survive a reload.

    Args:
        http_port: The local HTTP server's port.
        state: The temp state dir.
    """
    check = _check("tcp", http_port)
    results = runner.load_results()
    for _ in range(5):
        runner.run_and_record(check, results)
    assert len(runner.load_results()[check["id"]]) == 3


def test_seeded_once_then_deletable(state: Path) -> None:
    """First load writes the seed checks; a delete sticks across reloads.

    Args:
        state: The temp state dir.
    """
    checks = runner.load_checks()
    assert [c["id"] for c in checks] == [c["id"] for c in runner.SEED_CHECKS]
    runner.delete(checks, {}, "og-api-tags")
    assert "og-api-tags" not in {c["id"] for c in runner.load_checks()}


def test_check_state_is_not_served() -> None:
    """Check state lives under STATE_DIR, not DATA_DIR (served, so agent-readable)."""
    for path in REAL_PATHS:
        assert not path.startswith(config.DATA_DIR.rstrip("/") + "/")
