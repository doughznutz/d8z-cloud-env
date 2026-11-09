"""The LLM agent CLIs baked into carol's image, and how to run each headlessly.

Every agent is driven the same way: one prompt in as an argument, its final
answer read from stdout, working directory an empty scratch directory.

Agents read the docs exactly one way: curl against carol's doc server, as
plain text (``<page>?format=text``, server/docserver.py) - the same pages
a person browses. Each CLI's config (agent-config/) allows that and nothing
else. Underneath, this module only ever runs inside the separate
carol-agents container (server/agent_runner.py): it mounts no volumes at
all - no docs, no workspace source, no carol state - and gets the LLM keys
alone, and the CLIs run as an unprivileged user. Carol reaches it over HTTP
via server/agent_client.py.
"""

import os
import pwd
import re
import shutil
import signal
import subprocess
import time
import urllib.parse
import urllib.request
from collections.abc import Callable
from dataclasses import dataclass

import yaml

from .models import AgentOutcome


@dataclass(frozen=True)
class Agent:
    """One agent CLI: what it's called, how to invoke it, and what auth it needs.

    Attributes:
        name: Short key used in run records and NATS subjects.
        label: Display name for the dashboard.
        binary: Executable looked up on PATH.
        argv: Builds the full command line for one prompt.
        auth_env: Env vars any one of which authenticates it; empty if none needed.
        requires_url: A backend URL that must answer for it to work, with
            ``{ollama_url}`` filled in from carol.yaml (passed in by carol);
            empty if none.
    """

    name: str
    label: str
    binary: str
    argv: Callable[[str], list[str]]
    auth_env: tuple[str, ...] = ()
    requires_url: str = ""


#: Every agent the dashboard offers, keyed by name.
AGENTS: dict[str, Agent] = {
    a.name: a
    for a in (
        Agent(
            name="claude",
            label="Claude Code",
            binary="claude",
            # -p: print mode (one answer, then exit). Its limits live in
            # agent-config/claude-managed-settings.json (installed as
            # /etc/claude-code/managed-settings.json): Read only under /data,
            # no shell/edit/web; anything not allowed is denied, not prompted.
            argv=lambda p: ["claude", "-p", p],
            # CLAUDE_CODE_OAUTH_TOKEN comes from `claude setup-token` for a
            # subscription login; ANTHROPIC_API_KEY for API billing.
            auth_env=("ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"),
        ),
        Agent(
            name="gemini",
            label="Gemini CLI",
            binary="gemini",
            # Limits in agent-config/gemini-policy.toml (installed as
            # /etc/gemini-cli/policies/carol.toml): no shell/write/edit/web;
            # gemini itself rejects paths outside its workspace (cwd /data).
            argv=lambda p: ["gemini", "-p", p],
            auth_env=("GEMINI_API_KEY", "GOOGLE_API_KEY"),
        ),
        Agent(
            name="codex",
            label="Codex",
            binary="codex",
            # The scratch dir isn't a git repo, hence --skip-git-repo-check.
            # Sandbox, approvals and the key-free shell env are in
            # agent-config/codex-config.toml (~/.codex/config.toml); codex
            # has no per-command rules, so the carol-agents container
            # (nothing mounted) is what confines it.
            argv=lambda p: [
                "codex",
                "exec",
                "--skip-git-repo-check",
                "-C",
                AGENT_WORKDIR,
                p,
            ],
            auth_env=("OPENAI_API_KEY", "CODEX_API_KEY"),
        ),
        Agent(
            name="continue",
            label="Continue (cn → og)",
            binary="cn",
            # Local models via og's Ollama; run() writes the config first.
            # Tool policy (read/list/search only) in
            # agent-config/continue-permissions.yaml (~/.continue/).
            argv=lambda p: ["cn", "-p", p, "--config", _CONTINUE_CONFIG],
            requires_url="{ollama_url}/api/tags",
        ),
    )
}

#: Where cn's config is generated before each run.
_CONTINUE_CONFIG = "/tmp/carol-continue.yaml"


def _write_continue_config(ollama_url: str, models: list[str]) -> None:
    """Writes cn's config from carol.yaml's agents.continue settings.

    Args:
        ollama_url: og's Ollama API.
        models: Models cn may use; the first is its default.
    """
    doc = {
        "name": "Carol Ollama Gateway",
        "version": "0.0.1",
        "schema": "v1",
        "models": [
            {
                "name": m,
                "provider": "ollama",
                "model": m,
                "apiBase": ollama_url,
                "roles": ["chat"],
            }
            for m in models
        ],
    }
    with open(_CONTINUE_CONFIG, "w") as f:
        yaml.safe_dump(doc, f, sort_keys=False)


_ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07")


def availability(agent: Agent, ollama_url: str) -> tuple[bool, str]:
    """Reports whether an agent can run here, and why not if it can't.

    Args:
        agent: The agent to check.
        ollama_url: og's Ollama API, for agents that need it.

    Returns:
        (ok, detail): ok is False if the binary is missing or no auth env
        var is set or its backend is unreachable; detail names what's
        missing (or what auth was found).
    """
    if shutil.which(agent.binary) is None:
        return False, f"`{agent.binary}` not installed"
    if agent.requires_url:
        url = agent.requires_url.replace("{ollama_url}", ollama_url.rstrip("/"))
        host = urllib.parse.urlsplit(url).netloc
        try:
            with urllib.request.urlopen(url, timeout=2):
                pass
        except (OSError, ValueError) as e:
            return False, f"can't reach `{host}` ({getattr(e, 'reason', e)})"
    if not agent.auth_env:
        return True, "no key needed"
    found = [k for k in agent.auth_env if os.environ.get(k)]
    if not found:
        return False, "needs " + " or ".join(f"`{k}`" for k in agent.auth_env)
    return True, f"using `{found[0]}`"


#: Unprivileged user the CLIs run as (created in dev/carol/Dockerfile). It
#: can't read root-only paths such as /app, carol's own code, so the OS -
#: not just each CLI's config - keeps agents out of them.
AGENT_USER = "carol-agent"

#: That user's home, holding cn's and codex's configs (agent-config/).
AGENT_HOME = "/var/lib/carol-agent"

#: Every CLI's working directory: empty, and nothing to read in it - agents
#: reach the docs only over HTTP. (Gemini's workspace is this directory.)
AGENT_WORKDIR = "/var/lib/carol-agent/work"


def _drop_to_agent_user() -> bool:
    """Reports whether CLIs should run as AGENT_USER.

    Only when this process is root (it is, in carol-agents) and the user
    exists - so running agents.py elsewhere, e.g. a dev shell, still works.

    Returns:
        True if the CLI subprocess should switch to AGENT_USER.
    """
    if os.getuid() != 0:
        return False
    try:
        pwd.getpwnam(AGENT_USER)
    except KeyError:
        return False
    return True


def _env() -> dict[str, str]:
    """Builds the environment every agent subprocess runs with.

    Returns:
        A copy of this container's env (only the LLM keys arrive here), with
        HOME pointed at the agent user's (where its CLI configs live), plus
        no-colour hints, gemini's folder trust, and the API-key alias
        codex's exec mode looks for.
    """
    env = dict(os.environ)
    if _drop_to_agent_user():
        env.update(HOME=AGENT_HOME, USER=AGENT_USER, LOGNAME=AGENT_USER)
    env.setdefault("NO_COLOR", "1")
    env["TERM"] = "dumb"
    # Gemini refuses to run headless in a folder it hasn't been told to
    # trust; /data (its cwd) is ours.
    env["GEMINI_CLI_TRUST_WORKSPACE"] = "true"
    if env.get("OPENAI_API_KEY") and not env.get("CODEX_API_KEY"):
        env["CODEX_API_KEY"] = env["OPENAI_API_KEY"]
    return env


def _text(data: str | bytes | None) -> str:
    """Normalises captured process output to ANSI-free text.

    Args:
        data: Captured output, which TimeoutExpired may hand back as bytes.

    Returns:
        The decoded output with terminal escape sequences removed.
    """
    if data is None:
        return ""
    if isinstance(data, bytes):
        data = data.decode(errors="replace")
    return _ANSI.sub("", data).strip()


def run(
    agent: Agent, prompt: str, timeout: int, ollama_url: str, models: list[str]
) -> AgentOutcome:
    """Runs one agent CLI on one prompt, to completion or timeout.

    The CLI gets its own process group so that a timeout kills it *and*
    whatever node/tool subprocesses it spawned, not just the top process.

    Args:
        agent: The agent to run.
        prompt: The full prompt text.
        timeout: Seconds before the process group is killed.
        ollama_url: og's Ollama API (cn only).
        models: Ollama models cn may use (cn only).

    Returns:
        The agent's stdout as its answer, plus stderr, exit code and timing.
    """
    started = time.monotonic()
    os.makedirs(AGENT_WORKDIR, exist_ok=True)
    if agent.name == "continue":
        _write_continue_config(ollama_url, models)
    try:
        proc = subprocess.Popen(
            agent.argv(prompt),
            cwd=AGENT_WORKDIR,
            env=_env(),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=True,
            user=AGENT_USER if _drop_to_agent_user() else None,
            group=AGENT_USER if _drop_to_agent_user() else None,
            extra_groups=[] if _drop_to_agent_user() else None,
        )
    except OSError as e:
        return {
            "status": "failed",
            "output": "",
            "stderr": str(e),
            "exit_code": None,
            "duration_s": 0.0,
        }
    try:
        out, err = proc.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        out, err = proc.communicate()
        return {
            "status": "timeout",
            "output": _text(out),
            "stderr": _text(err) + f"\n[killed after {timeout}s]",
            "exit_code": None,
            "duration_s": round(time.monotonic() - started, 1),
        }
    return {
        "status": "ok" if proc.returncode == 0 else "failed",
        "output": _text(out),
        "stderr": _text(err),
        "exit_code": proc.returncode,
        "duration_s": round(time.monotonic() - started, 1),
    }
