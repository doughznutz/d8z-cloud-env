"""Gopher's side of the gopher-agents service (server/agent_runner.py).

Agents never run in gopher's own container - it has the workspace source
mounted - so every availability check and run goes over HTTP to
gopher-agents, which can only see the generated docs.
"""

import json
import urllib.error
import urllib.parse
import urllib.request
from typing import cast

from . import config, settings
from .agents import AGENTS  # names only; nothing is run in gopher
from .models import AgentOutcome


def availability() -> dict[str, tuple[bool, str]]:
    """Asks gopher-agents which agents can run, and why not if they can't.

    Returns:
        (ok, detail) per agent name. If gopher-agents itself is unreachable,
        every agent is reported unavailable with that reason.
    """
    query = urllib.parse.urlencode({"ollama_url": settings.load().ollama_url})
    try:
        with urllib.request.urlopen(f"{config.AGENTS_URL}/agents?{query}", timeout=10) as r:
            raw = cast(dict[str, list[object]], json.load(r))
        return {n: (bool(v[0]), str(v[1])) for n, v in raw.items()}
    except (OSError, ValueError) as e:
        why = f"gopher-agents unreachable at {config.AGENTS_URL} ({getattr(e, 'reason', e)})"
        return {n: (False, why) for n in AGENTS}


def run(name: str, prompt: str) -> AgentOutcome:
    """Runs one agent in gopher-agents and waits for its outcome.

    Args:
        name: The agent's key.
        prompt: The full prompt text.

    Returns:
        The agent's outcome, or a "failed" outcome describing why
        gopher-agents couldn't be reached.
    """
    s = settings.load()
    body = json.dumps(
        {
            "agent": name,
            "prompt": prompt,
            "timeout": s.agent_timeout,
            "ollama_url": s.ollama_url,
            "models": list(s.continue_models),
        }
    ).encode()
    req = urllib.request.Request(
        f"{config.AGENTS_URL}/run",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        # gopher-agents enforces the agent's own timeout; this just leaves
        # it room to report back.
        with urllib.request.urlopen(req, timeout=s.agent_timeout + 60) as r:
            return cast(AgentOutcome, json.load(r))
    except (OSError, ValueError) as e:
        detail = e.read().decode(errors="replace") if isinstance(e, urllib.error.HTTPError) else ""
        return {
            "status": "failed",
            "output": "",
            "stderr": f"gopher-agents at {config.AGENTS_URL}: {e} {detail}".strip(),
            "exit_code": None,
            "duration_s": 0.0,
        }
