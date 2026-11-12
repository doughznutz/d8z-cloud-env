"""Typed result shapes shared between the extractors and the orchestrator.

Every extractor returns the same `ExtractResult` shape regardless of which
language tool it wrapped, which is what lets the orchestrator (and, later,
a NATS job handler) treat "go", "typescript" and "python" interchangeably.
"""

from typing import Literal, TypedDict


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


#: The probe types a check can be; each has a runner in server/checks.
CheckKind = Literal["http", "tcp"]


class Check(TypedDict):
    """One user-defined check against a cluster container.

    Hosts and ports are the target's own compose service name and container
    port: probes run from inside gopher over the networks it shares with its
    targets, so nothing needs publishing to the host. The http-only fields
    are stored (and ignored) on tcp checks too, so every check has one shape.
    """

    id: str
    name: str
    kind: CheckKind
    host: str
    port: int
    path: str
    method: str
    timeout_s: int
    expect_status: list[int]
    expect_contains: str
    enabled: bool


class RunOutcome(TypedDict):
    """The pass/fail verdict of one probe, before it's timestamped."""

    ok: bool
    latency_ms: float
    detail: str


class CheckResult(RunOutcome):
    """A `RunOutcome` plus the UTC time the probe ran, as stored in history."""

    ts: str


#: Persisted check history, keyed by check id.
CheckHistory = dict[str, list[CheckResult]]


class ChatMessage(TypedDict):
    """One chat message, as read back from the shared CHAT stream."""

    seq: int
    sender: str
    text: str
    ts: str


#: Lifecycle of one agent's part of a report run. "interrupted" means the
#: run was still going when gopher's server process went away.
AgentStatus = Literal["running", "ok", "failed", "timeout", "interrupted"]


class AgentOutcome(TypedDict):
    """What running one agent CLI to completion produced."""

    status: AgentStatus
    output: str
    stderr: str
    exit_code: int | None
    duration_s: float


class AgentResult(AgentOutcome):
    """One agent's slot in a report run, plus whether it reached the NATS bus.

    `published` is None when publishing wasn't requested (or hasn't happened
    yet), True/False once it was attempted; `publish_detail` says where it
    went or why it failed.
    """

    agent: str
    started: str
    published: bool | None
    publish_detail: str


class ReportRun(TypedDict):
    """One prompt sent to a set of agents at once, and each agent's response."""

    id: str
    ts: str
    title: str
    prompt: str
    publish: bool
    results: dict[str, AgentResult]


class Prompt(TypedDict):
    """A named prompt preset."""

    title: str
    text: str


class BusReport(TypedDict):
    """One agent response as read back from the REPORTS stream."""

    seq: int
    subject: str
    run_id: str
    agent: str
    title: str
    status: str
    output: str
    ts: str
