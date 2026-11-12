"""Gopher's user settings, kept in ``~/.d8z/gopher.yaml`` on the host.

That one file (and only it - the rest of ~/.d8z holds secrets) is bind
mounted into every gopher on this machine, root and workspace alike, at
CONFIG_FILE, so prompts and settings survive any container, volume or
system restart - same home as the gateway's workspace.yaml and
deployment.yaml. Per-instance wiring (paths, base URL, NATS URL) stays in
the environment, set by compose / gw-server; everything a person would
want to tune lives here.

The file is re-read whenever it changes, so hand edits apply on the next
page load. Writes go to the file *in place*: a single-file bind mount
can't be replaced by rename, which is how everything else here saves.
"""

import json
import os
import threading
from dataclasses import dataclass, field, replace
from typing import TypeVar, cast

import yaml

from . import config
from .models import Prompt

_lock = threading.Lock()

T = TypeVar("T")

#: The default context of older carols, which pointed agents at files - a
#: gopher.yaml (moved from carol.yaml) still holding exactly this is upgraded to DEFAULT_CONTEXT.
OLD_DEFAULT_CONTEXT = (
    "You are reviewing generated API documentation for a software workspace. It is in "
    "{data_dir}, one subdirectory per language: go/ (go doc output), python/ (pdoc HTML for "
    "packages, plus a page per standalone script), typescript/ (typedoc output) and "
    "shell/ (each script's comments and source). Read those files to answer; do not "
    "modify anything. Reply with your report in Markdown."
)

#: Prepended to every report prompt; {docs_url} becomes config.DOCS_URL.
DEFAULT_CONTEXT = (
    "You are reviewing the generated API documentation for a software workspace. You can "
    "read it only through its documentation server, as plain text, using exactly this "
    "command form: curl -s '<url>'\n\n"
    "Every page is at {docs_url}<path>?format=text, and every link in a page is "
    "such a <path> (e.g. python/index.html). Start from each language's index: go/, "
    "python/, typescript/ and shell/ - e.g. curl -s '{docs_url}go/?format=text'. You "
    "have no other access to files, and must not try to modify anything. Reply with "
    "your report in Markdown."
)

#: Presets a brand-new gopher.yaml starts with.
DEFAULT_PROMPTS: tuple[Prompt, ...] = (
    {
        "title": "Documentation overview",
        "text": (
            "Survey the generated documentation and write a report that lists every "
            "documented module/package per language, summarises what each one does in "
            "one or two sentences, and describes how they appear to fit together."
        ),
    },
    {
        "title": "Documentation gaps",
        "text": (
            "Find public APIs (functions, types, methods, classes) whose generated "
            "documentation is missing, empty, or too vague to be useful. Report them "
            "grouped by language and package, worst first, with a one-line suggestion "
            "for each."
        ),
    },
    {
        "title": "API consistency review",
        "text": (
            "Review the documented APIs for inconsistencies: naming conventions, "
            "parameter ordering, error handling patterns, and duplicated concepts across "
            "packages or languages. Report concrete examples and recommend fixes."
        ),
    },
    {
        "title": "Onboarding guide",
        "text": (
            "Using only the generated documentation, write an onboarding guide for a new "
            "developer: the main components, the key entry points to read first, and the "
            "concepts they need to understand before making a change."
        ),
    },
)


@dataclass(frozen=True)
class Settings:
    """Everything in gopher.yaml, with defaults filled in for anything missing.

    Attributes:
        agent_timeout: Seconds one agent CLI may run before it's killed.
        ollama_url: og's Ollama API, which cn is pointed at.
        continue_models: Ollama models cn may use; the first is its default.
        docs_keep_runs: Doc-generation runs kept per language.
        checks_keep_runs: Results kept per check.
        reports_keep_runs: Report runs kept before the oldest are deleted.
        report_subject: NATS subject prefix responses publish to.
        bus_history: Most bus reports shown at once.
        chat_history: Most chat messages shown at once.
        prompt_context: Prepended to every report prompt.
        prompts: Prompt presets, in display order.
        problems: Anything in the file that was invalid and replaced by a
            default, for the dashboard to show.
        persistent: Whether the file is the host's (bind mounted) copy; if
            not, changes only last as long as this container.
    """

    agent_timeout: int = 900
    ollama_url: str = "http://og:11434"
    continue_models: tuple[str, ...] = ("llama3.2:3b", "qwen2.5-coder:1.5b")
    docs_keep_runs: int = 25
    checks_keep_runs: int = 25
    reports_keep_runs: int = 50
    report_subject: str = "gopher.reports"
    bus_history: int = 25
    chat_history: int = 50
    prompt_context: str = DEFAULT_CONTEXT
    prompts: tuple[Prompt, ...] = DEFAULT_PROMPTS
    problems: tuple[str, ...] = field(default=(), compare=False)
    persistent: bool = field(default=False, compare=False)


_cache: tuple[tuple[float, int], Settings] | None = None


def _is_mounted(path: str) -> bool:
    """Reports whether `path` is itself a mount point (i.e. the host's file).

    Args:
        path: Absolute path inside this container.

    Returns:
        True if /proc/self/mountinfo lists it as a mount point.
    """
    try:
        with open("/proc/self/mountinfo") as f:
            return any(line.split()[4] == path for line in f if len(line.split()) > 4)
    except OSError:
        return False


def _pick(raw: dict[str, object], keys: str, kind: type[T], default: T, problems: list[str]) -> T:
    """Reads one dotted key from the parsed file, falling back if it's missing or wrong.

    Args:
        raw: The parsed YAML document.
        keys: Dotted path, e.g. "agents.timeout_seconds".
        kind: The type the value must have.
        default: Used when it's absent or invalid.
        problems: Collects a message for each invalid value.

    Returns:
        The file's value, or `default`.
    """
    node: object = raw
    for k in keys.split("."):
        if not isinstance(node, dict) or k not in node:
            return default
        node = node[k]
    if kind is int and isinstance(node, bool):  # bool is an int subclass
        node = None
    if not isinstance(node, kind) or (kind is int and isinstance(node, int) and node <= 0):
        problems.append(f"`{keys}` must be a {'positive ' if kind is int else ''}{kind.__name__}")
        return default
    return node


def _upgrade_context(context: str) -> str:
    """Replaces the old file-reading default context with the current one.

    A context someone edited is left alone; only the untouched old default
    (which told agents to read files they can no longer reach) is swapped.

    Args:
        context: prompt_context as read from gopher.yaml.

    Returns:
        The context to use.
    """
    return DEFAULT_CONTEXT if context == OLD_DEFAULT_CONTEXT else context


def _parse(raw: object) -> Settings:
    """Builds Settings from a parsed gopher.yaml, defaulting anything invalid.

    Args:
        raw: Whatever yaml.safe_load returned.

    Returns:
        The settings (with `problems` set for anything that was replaced).
    """
    problems: list[str] = []
    if not isinstance(raw, dict):
        return Settings(problems=("top level must be a mapping; using defaults",))
    d = Settings()
    models = _pick(raw, "agents.continue.models", list, list(d.continue_models), problems)
    prompts_raw = _pick(raw, "prompts", list, list(d.prompts), problems)
    prompts: list[Prompt] = []
    for i, p in enumerate(prompts_raw):
        if (
            isinstance(p, dict)
            and isinstance(p.get("title"), str)
            and isinstance(p.get("text"), str)
        ):
            prompts.append({"title": p["title"], "text": p["text"].strip()})
        else:
            problems.append(f"`prompts[{i}]` needs a string `title` and `text`; skipped")
    return Settings(
        agent_timeout=_pick(raw, "agents.timeout_seconds", int, d.agent_timeout, problems),
        ollama_url=_pick(raw, "agents.continue.ollama_url", str, d.ollama_url, problems),
        continue_models=tuple(str(m) for m in models),
        docs_keep_runs=_pick(raw, "docs.keep_runs", int, d.docs_keep_runs, problems),
        checks_keep_runs=_pick(raw, "checks.keep_runs", int, d.checks_keep_runs, problems),
        reports_keep_runs=_pick(raw, "reports.keep_runs", int, d.reports_keep_runs, problems),
        report_subject=_pick(raw, "reports.nats_subject", str, d.report_subject, problems),
        bus_history=_pick(raw, "reports.bus_history", int, d.bus_history, problems),
        chat_history=_pick(raw, "chat.history", int, d.chat_history, problems),
        prompt_context=_upgrade_context(
            _pick(raw, "prompt_context", str, d.prompt_context, problems).strip()
        ),
        prompts=tuple(prompts),
        problems=tuple(problems),
    )


def _scalar(value: str | int) -> str:
    """Renders one YAML scalar, quoted only when it has to be.

    Args:
        value: A str/int.

    Returns:
        The inline YAML for it.
    """
    out = yaml.safe_dump(value, default_flow_style=True, allow_unicode=True, width=1 << 20)
    return out.removesuffix("\n").removesuffix("\n...").strip()


def _text(value: str, indent: int) -> str:
    """Renders a string as a readable ``|`` block when it safely can.

    Args:
        value: The text.
        indent: Spaces to indent the block's lines by.

    Returns:
        YAML for the value, starting right after ``key:``.
    """
    lines = value.split("\n")
    plain = (
        value
        and value == value.strip()
        and "\r" not in value
        and "\t" not in value
        and all(line == line.rstrip() for line in lines)
    )
    if not plain:
        return " " + _scalar(value)
    pad = " " * indent
    return " |-\n" + "\n".join(pad + line if line else "" for line in lines)


def render(s: Settings) -> str:
    """Renders settings as the commented, hand-editable gopher.yaml.

    Args:
        s: The settings to write.

    Returns:
        The full file text.
    """
    models = "\n".join(f"      - {_scalar(m)}" for m in s.continue_models) or "      []"
    prompts = (
        "\n".join(
            f"  - title: {_scalar(p['title'])}\n    text:{_text(p['text'], 6)}" for p in s.prompts
        )
        if s.prompts
        else "  []"
    )
    text = f"""# Gopher settings - shared by every gopher on this machine (the root one and
# each workspace's). Edit by hand or from gopher's dashboard: gopher re-reads
# this file when it changes, so edits apply on the next page load. Saving a
# prompt from the dashboard rewrites the file in this layout, so comments you
# add by hand are not kept.

agents:
  # Seconds one agent CLI may run before it is killed.
  timeout_seconds: {_scalar(s.agent_timeout)}
  continue:
    # og's Ollama API, which the Continue CLI (cn) is pointed at.
    ollama_url: {_scalar(s.ollama_url)}
    # Ollama models cn may use; the first one is its default.
    models:
{models}

docs:
  # Doc-generation runs kept per language (Docs tab history).
  keep_runs: {_scalar(s.docs_keep_runs)}

checks:
  # Results kept per check (Tests tab history).
  keep_runs: {_scalar(s.checks_keep_runs)}

reports:
  # Report runs kept before the oldest are deleted.
  keep_runs: {_scalar(s.reports_keep_runs)}
  # Responses publish to "<nats_subject>.<agent>" when asked to.
  nats_subject: {_scalar(s.report_subject)}
  # Most published reports shown in the Reports tab's bus view.
  bus_history: {_scalar(s.bus_history)}

chat:
  # Most chat messages shown in the Chat tab.
  history: {_scalar(s.chat_history)}

# Prepended to every report prompt; {{docs_url}} becomes this gopher's docs URL.
prompt_context:{_text(s.prompt_context, 2)}

# Prompt presets offered in the Reports tab, in this order.
prompts:
{prompts}
"""
    # Belt and braces: never write a file that wouldn't read back the same.
    if _parse(yaml.safe_load(text)) != s:
        raise ValueError("gopher.yaml didn't round-trip; refusing to write it")
    return text


def _legacy_prompts() -> list[Prompt]:
    """Reads presets saved by older carols (before carol.yaml), to migrate them.

    Returns:
        The presets from STATE_DIR/prompts.json, or none.
    """
    # entrypoint.sh copies carol's old prompts.json into STATE_DIR on startup.
    path = os.path.join(config.STATE_DIR, "prompts.json")
    try:
        with open(path) as f:
            return cast(list[Prompt], json.load(f))
    except (OSError, ValueError):
        return []


def _write(s: Settings) -> None:
    """Writes the file in place (a bind-mounted single file can't be renamed over).

    Args:
        s: The settings to write.
    """
    text = render(s)
    os.makedirs(os.path.dirname(config.CONFIG_FILE), exist_ok=True)
    with open(config.CONFIG_FILE, "w") as f:
        f.write(text)


def load() -> Settings:
    """Returns the current settings, re-reading gopher.yaml if it changed.

    A missing or empty file is created with the defaults (plus any presets
    an older carol had saved in its state). A file that doesn't parse
    is never overwritten - defaults are used and the error is reported.

    Returns:
        The settings.
    """
    global _cache
    path = config.CONFIG_FILE
    with _lock:
        if os.path.isdir(path):
            # Docker made a directory because the host file didn't exist (or
            # this daemon can't see the host's ~/.d8z, e.g. a dind workspace).
            return Settings(
                problems=(f"{path} is a directory, not a file - using defaults",),
            )
        try:
            st = os.stat(path)
            key = (st.st_mtime, st.st_size)
        except OSError:
            st, key = None, (0.0, 0)
        if _cache and _cache[0] == key and st is not None:
            return _cache[1]
        persistent = _is_mounted(path)
        if st is None or st.st_size == 0:
            legacy = [p for p in _legacy_prompts() if isinstance(p, dict)]
            titles = {p["title"] for p in legacy}
            seeded = Settings(
                prompts=tuple(p for p in DEFAULT_PROMPTS if p["title"] not in titles)
                + tuple(legacy)
            )
            try:
                _write(seeded)
            except OSError as e:
                return replace(seeded, problems=(f"couldn't create {path}: {e}",))
            st = os.stat(path)
            key = (st.st_mtime, st.st_size)
            s = replace(seeded, persistent=persistent)
        else:
            try:
                with open(path) as f:
                    s = replace(_parse(yaml.safe_load(f)), persistent=persistent)
            except (OSError, yaml.YAMLError) as e:
                return Settings(problems=(f"couldn't read {path} ({e}); using defaults",))
        _cache = (key, s)
        return s


def update_prompts(prompts: list[Prompt]) -> None:
    """Replaces the preset list, keeping every other setting as it is on disk.

    Re-reads the file first, so a change made meanwhile (by hand, or by
    another gopher sharing the file) isn't lost.

    Args:
        prompts: The new preset list, in display order.

    Raises:
        ValueError: If the file on disk is unreadable - it's left untouched
            rather than replaced with defaults.
    """
    global _cache
    with _lock:
        _cache = None
    current = load()
    if any("couldn't read" in p or "is a directory" in p for p in current.problems):
        raise ValueError("; ".join(current.problems))
    with _lock:
        _write(replace(current, prompts=tuple(prompts), problems=()))
        _cache = None
