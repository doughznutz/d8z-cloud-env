"""HTTP checks: request a path on the target and judge the response."""

import time

import requests

from ..models import Check, RunOutcome


def probe(check: Check) -> RunOutcome:
    """Requests the check's URL and judges the response.

    A check with no ``expect_status`` only needs the request not to fail
    ("reachable, whatever it says"); one with ``expect_status`` also needs
    the response code in that list, and one with ``expect_contains`` needs
    that substring in the body as well.

    Args:
        check: The HTTP check to run.

    Returns:
        The outcome of the probe.
    """
    url = f"http://{check['host']}:{check['port']}{check.get('path') or '/'}"
    t0 = time.monotonic()
    try:
        resp = requests.request(
            check.get("method") or "GET", url, timeout=check.get("timeout_s", 5)
        )
    except requests.RequestException as e:
        return {"ok": False, "latency_ms": _since(t0), "detail": str(e)}
    latency_ms = _since(t0)
    expect = check.get("expect_status") or []
    ok = resp.status_code in expect if expect else True
    detail = f"HTTP {resp.status_code}"
    contains = check.get("expect_contains")
    if ok and contains and contains not in resp.text:
        ok = False
        detail += " (response missing expected text)"
    return {"ok": ok, "latency_ms": latency_ms, "detail": detail}


def _since(t0: float) -> float:
    """Milliseconds since ``t0``, to one decimal place.

    Args:
        t0: A `time.monotonic()` reading.

    Returns:
        The elapsed time in ms.
    """
    return round((time.monotonic() - t0) * 1000, 1)
