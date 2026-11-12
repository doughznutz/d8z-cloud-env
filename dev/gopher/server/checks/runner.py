"""Defines, runs and records checks - the checks' counterpart to orchestrator.py.

Check definitions and their run history live under STATE_DIR (never
DATA_DIR, which is served, and so readable by the report agents). Like the
orchestrator, nothing here knows about Streamlit, so a future NATS job
handler can run checks through the same functions the dashboard does.
"""

import uuid
from datetime import datetime, timezone
from typing import cast

from .. import config, settings, store
from ..models import Check, CheckHistory, CheckKind, CheckResult, RunOutcome
from . import REGISTRY


def _seed(check_id: str, name: str, host: str, port: int, path: str = "/") -> Check:
    """Builds one of the first-boot HTTP checks.

    Args:
        check_id: Its fixed id.
        name: Its display name.
        host: Target service name.
        port: Target container port.
        path: Path to request.

    Returns:
        The check.
    """
    return {
        "id": check_id,
        "name": name,
        "kind": "http",
        "host": host,
        "port": port,
        "path": path,
        "method": "GET",
        "timeout_s": 5,
        "expect_status": [200] if path != "/" else [],
        "expect_contains": "",
        "enabled": True,
    }


#: Written on first boot only - this project's own dev services, reachable
#: by their compose service name on internal_net.
SEED_CHECKS: tuple[Check, ...] = (
    _seed("dood-reachable", "Dood gateway responds", "dood", 80),
    _seed("dozzle-reachable", "Dozzle UI responds", "dozzle", 8080),
    _seed("deightz-reachable", "Deightz code-server responds", "deightz", 8443),
    _seed("og-dashboard-reachable", "Ollama gateway dashboard responds", "og", 8501),
    _seed("og-api-tags", "Ollama API lists models", "og", 11434, "/api/tags"),
)


def load_checks() -> list[Check]:
    """Loads the saved check definitions, writing the seed set on first boot.

    Returns:
        Every defined check, in display order.
    """
    checks = store.load_json(config.CHECKS_FILE, None)
    if checks is None:
        checks = [dict(c) for c in SEED_CHECKS]
        store.save_json(config.CHECKS_FILE, checks)
    return cast(list[Check], checks)


def save_checks(checks: list[Check]) -> None:
    """Persists the full list of check definitions.

    Args:
        checks: Every defined check, in display order.
    """
    store.save_json(config.CHECKS_FILE, checks)


def load_results() -> CheckHistory:
    """Loads saved run history.

    Returns:
        Each check's past results, keyed by check id.
    """
    return cast(CheckHistory, store.load_json(config.RESULTS_FILE, {}))


def new_check(
    name: str,
    kind: str,
    host: str,
    port: int,
    path: str = "/",
    method: str = "GET",
    timeout_s: int = 5,
    expect_status: str = "",
    expect_contains: str = "",
) -> Check:
    """Validates form input and builds a new, enabled check with a fresh id.

    Args:
        name: Display name.
        kind: A key of `REGISTRY`.
        host: Target service/container name.
        port: Target port.
        path: HTTP path (http only).
        method: HTTP method (http only).
        timeout_s: Probe timeout in seconds.
        expect_status: Comma-separated status codes; blank = just reachable.
        expect_contains: Text the response body must contain (http only).

    Returns:
        The check, not yet saved.

    Raises:
        ValueError: If any field is invalid; the message says which.
    """
    name, host = name.strip(), host.strip()
    if not name or not host:
        raise ValueError("Name and host are required.")
    if kind not in REGISTRY:
        raise ValueError(f"Unknown check kind {kind!r}; known: {', '.join(REGISTRY)}.")
    if not 1 <= port <= 65535:
        raise ValueError("Port must be between 1 and 65535.")
    try:
        codes = [int(s) for s in expect_status.split(",") if s.strip()]
    except ValueError:
        raise ValueError(
            "Expected status codes must be a comma-separated list of numbers."
        ) from None
    return {
        "id": uuid.uuid4().hex[:8],
        "name": name,
        "kind": cast(CheckKind, kind),
        "host": host,
        "port": int(port),
        "path": path or "/",
        "method": method or "GET",
        "timeout_s": int(timeout_s),
        "expect_status": codes,
        "expect_contains": expect_contains,
        "enabled": True,
    }


def run_check(check: Check) -> CheckResult:
    """Runs one check through its kind's prober and timestamps the outcome.

    Args:
        check: The check to run.

    Returns:
        The timestamped result (not yet recorded).
    """
    prober = REGISTRY.get(check["kind"])
    outcome: RunOutcome
    if prober is None:
        outcome = {"ok": False, "latency_ms": 0.0, "detail": f"unknown kind {check['kind']!r}"}
    else:
        outcome = prober.probe(check)
    return {**outcome, "ts": datetime.now(timezone.utc).isoformat(timespec="seconds")}


def record(results: CheckHistory, check_id: str, result: CheckResult) -> None:
    """Appends a result to a check's history, trims it, and persists it.

    Args:
        results: The full history, updated in place.
        check_id: The check the result belongs to.
        result: The result to append.
    """
    hist = results.setdefault(check_id, [])
    hist.append(result)
    del hist[: -settings.load().checks_keep_runs]
    store.save_json(config.RESULTS_FILE, results)


def run_and_record(check: Check, results: CheckHistory) -> CheckResult:
    """Runs one check and records its result.

    Args:
        check: The check to run.
        results: The full history, updated in place.

    Returns:
        The recorded result.
    """
    result = run_check(check)
    record(results, check["id"], result)
    return result


def run_enabled(checks: list[Check], results: CheckHistory) -> dict[str, CheckResult]:
    """Runs every enabled check and records each result.

    Args:
        checks: The defined checks.
        results: The full history, updated in place.

    Returns:
        Each run check's result, keyed by check id.
    """
    return {c["id"]: run_and_record(c, results) for c in checks if c.get("enabled", True)}


def delete(checks: list[Check], results: CheckHistory, check_id: str) -> None:
    """Removes a check and its history, and persists both.

    Args:
        checks: The defined checks, updated in place.
        results: The full history, updated in place.
        check_id: The check to remove.
    """
    checks[:] = [c for c in checks if c["id"] != check_id]
    results.pop(check_id, None)
    save_checks(checks)
    store.save_json(config.RESULTS_FILE, results)
