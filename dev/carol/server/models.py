"""Typed result shapes shared between the extractors and the orchestrator.

Every extractor returns the same `ExtractResult` shape regardless of which
language tool it wrapped, which is what lets the orchestrator (and, later,
a NATS job handler) treat "go", "typescript" and "python" interchangeably.
"""

from typing import TypedDict


class Target(TypedDict, total=False):
    """One documented unit within a run: a Go module, TS project, or Python package.

    `total=False` because each extractor only fills the keys relevant to it
    (e.g. golang sets "module"/"package", typescript sets "project") - there
    is no single unit of work shared by all three languages.
    """

    module: str
    project: str
    package: str
    ok: bool
    detail: str
    file: str
    out_dir: str


class ExtractResult(TypedDict):
    """What every extractor's `extract()` function returns."""

    ok: bool
    detail: str
    targets: list[Target]


class RunRecord(ExtractResult):
    """An `ExtractResult` plus the timestamp the orchestrator stamps on it before saving."""

    ts: str


#: Persisted run history, keyed by language name ("go", "typescript", "python").
History = dict[str, list[RunRecord]]


class ChatMessage(TypedDict):
    """One chat message, as read back from the shared CHAT stream."""

    seq: int
    sender: str
    text: str
    ts: str
