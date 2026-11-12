"""Fans one prompt out to several agent CLIs at once and records each response.

A run's agents execute on background threads in gopher's server process, not
in a Streamlit script run, so closing the browser or navigating away never
kills one. Every state change is written straight to that run's JSON file
under REPORTS_DIR; the dashboard only ever reads those files back.
"""

import asyncio
import copy
import glob
import json
import os
import secrets
import threading
from datetime import datetime, timezone
from typing import cast

from . import agent_client, agents, bus, config, settings
from .models import ReportRun

#: Guards `_active` and every write of a run file.
_lock = threading.Lock()

#: Runs with at least one agent still going in *this* process, by id.
_active: dict[str, ReportRun] = {}


def _now() -> str:
    """Returns the current UTC time as an ISO-8601 string.

    Returns:
        The timestamp, to the second.
    """
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _path(run_id: str) -> str:
    """Returns where a run's JSON file lives.

    Args:
        run_id: The run's id.

    Returns:
        The file path under REPORTS_DIR.
    """
    return os.path.join(config.REPORTS_DIR, f"{run_id}.json")


def _save(run: ReportRun) -> None:
    """Persists a run atomically. Callers must hold `_lock`.

    Args:
        run: The run to write.
    """
    os.makedirs(config.REPORTS_DIR, exist_ok=True)
    tmp = _path(run["id"]) + ".tmp"
    with open(tmp, "w") as f:
        json.dump(run, f, indent=2)
    os.replace(tmp, _path(run["id"]))


def _prune() -> None:
    """Deletes the oldest finished run files beyond gopher.yaml's reports.keep_runs."""
    files = sorted(glob.glob(os.path.join(config.REPORTS_DIR, "*.json")))
    for f in files[: -settings.load().reports_keep_runs]:
        if os.path.basename(f)[: -len(".json")] not in _active:
            os.remove(f)


def start(title: str, prompt: str, agent_names: list[str], publish: bool) -> str:
    """Starts one run: the same prompt sent to every named agent in parallel.

    Args:
        title: Short label for the run (usually the preset name).
        prompt: The full prompt every agent receives.
        agent_names: Keys into `agents.AGENTS`.
        publish: Also publish each response to the NATS bus as it finishes.

    Returns:
        The new run's id; poll it with `load`.

    Raises:
        ValueError: If no agents are given or one isn't known.
    """
    if not agent_names:
        raise ValueError("no agents selected")
    unknown = [n for n in agent_names if n not in agents.AGENTS]
    if unknown:
        raise ValueError(f"unknown agent(s): {', '.join(unknown)}")
    # Timestamp-first so a plain sort of the file names is chronological.
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + secrets.token_hex(3)
    started = _now()
    run: ReportRun = {
        "id": run_id,
        "ts": started,
        "title": title,
        "prompt": prompt,
        "publish": publish,
        "results": {
            n: {
                "agent": n,
                "status": "running",
                "output": "",
                "stderr": "",
                "exit_code": None,
                "duration_s": 0.0,
                "started": started,
                "published": None,
                "publish_detail": "",
            }
            for n in agent_names
        },
    }
    with _lock:
        _active[run_id] = run
        _save(run)
        _prune()
    for n in agent_names:
        threading.Thread(target=_run_agent, args=(run_id, n), daemon=True).start()
    return run_id


def _run_agent(run_id: str, name: str) -> None:
    """Thread body: runs one agent for one run, records it, optionally publishes it.

    Args:
        run_id: The run this agent belongs to.
        name: The agent's key.
    """
    with _lock:
        run = _active[run_id]
        prompt, title, publish = run["prompt"], run["title"], run["publish"]
    outcome = agent_client.run(name, prompt)
    with _lock:
        run["results"][name].update(outcome)
        _save(run)
    published: bool | None = None
    detail = ""
    if publish:
        try:
            subject, seq = asyncio.run(
                bus.publish_report(run_id, name, title, outcome["status"], outcome["output"])
            )
            published, detail = True, f"{subject} (stream seq {seq})"
        except Exception as e:  # recorded on the run and shown, not swallowed
            published, detail = False, str(e)
    with _lock:
        run["results"][name]["published"] = published
        run["results"][name]["publish_detail"] = detail
        _save(run)
        if all(r["status"] != "running" for r in run["results"].values()):
            del _active[run_id]


def is_running(run_id: str) -> bool:
    """Reports whether any of a run's agents are still going.

    Args:
        run_id: The run's id.

    Returns:
        True while the run is active in this process.
    """
    with _lock:
        return run_id in _active


def load(run_id: str) -> ReportRun | None:
    """Returns a snapshot of one run, live if it's still active.

    A run file left with "running" agents by a process that's gone (e.g. a
    container restart mid-run) is reported with those agents "interrupted".

    Args:
        run_id: The run's id.

    Returns:
        A copy of the run, or None if it doesn't exist.
    """
    with _lock:
        if run_id in _active:
            return copy.deepcopy(_active[run_id])
    path = _path(run_id)
    if not os.path.exists(path):
        return None
    with open(path) as f:
        run = cast(ReportRun, json.load(f))
    for r in run["results"].values():
        if r["status"] == "running":
            r["status"] = "interrupted"
    return run


def list_ids() -> list[str]:
    """Returns every stored run's id, newest first.

    Returns:
        Run ids.
    """
    files = glob.glob(os.path.join(config.REPORTS_DIR, "*.json"))
    return sorted((os.path.basename(f)[: -len(".json")] for f in files), reverse=True)
