"""Per-kind check backends, the checks' counterpart to server/extractors.

Each module exposes `probe(check) -> RunOutcome`. That one-function
contract is what lets runner.py drive any kind identically: add a kind by
adding a module here and its name to models.CheckKind.
"""

from typing import Protocol

from ..models import Check, CheckKind, RunOutcome
from . import http, tcp


class Prober(Protocol):
    """Structural shape every per-kind check module must satisfy."""

    def probe(self, check: Check) -> RunOutcome:
        """Runs one check and reports whether it passed."""
        ...


#: Check kind -> the module that runs it.
REGISTRY: dict[CheckKind, Prober] = {
    "http": http,
    "tcp": tcp,
}
