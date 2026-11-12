"""TCP checks: pass if a connection to host:port can be opened."""

import socket
import time

from ..models import Check, RunOutcome


def probe(check: Check) -> RunOutcome:
    """Opens (and immediately closes) a TCP connection to the target.

    Args:
        check: The TCP check to run.

    Returns:
        The outcome of the probe.
    """
    t0 = time.monotonic()
    try:
        with socket.create_connection(
            (check["host"], check["port"]), timeout=check.get("timeout_s", 5)
        ):
            pass
        ok, detail = True, "connected"
    except OSError as e:
        ok, detail = False, str(e)
    return {"ok": ok, "latency_ms": round((time.monotonic() - t0) * 1000, 1), "detail": detail}
