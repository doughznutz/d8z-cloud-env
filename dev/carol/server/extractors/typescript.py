"""Extracts TypeScript API docs with typedoc.

A workspace can hold several TS projects (this one does: the console app
under dev/dood/console is its own project), so every tsconfig.json under
source_dir is treated as its own project root and documented independently.

Real projects split a root tsconfig.json (often just `references` to child
configs, e.g. Vite's app/node split) from the config that actually lists
source files. This follows one level of `references` to find that config
and its `include`/`files`, falling back to "the whole project directory"
when a tsconfig has neither. --skipErrorChecking keeps a type error in the
project (unrelated to doc generation) from blocking doc output entirely.
"""

import json
import os
import subprocess
from typing import Any, cast

from ..models import ExtractResult, Target

#: Directories to never walk into: VCS metadata, installed packages, and
#: build output that would otherwise be documented as if it were source.
SKIP_DIRS = {".git", "node_modules", "dist", "build"}


def find_projects(source_dir: str) -> list[str]:
    """Finds every directory under `source_dir` that contains a tsconfig.json.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        Absolute paths of each project root found, in walk order.
    """
    projects: list[str] = []
    for root, dirs, files in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        if "tsconfig.json" in files:
            projects.append(root)
    return projects


def detect(source_dir: str) -> bool:
    """Reports whether `source_dir` contains any TypeScript projects at all.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        True if at least one tsconfig.json file was found.
    """
    return bool(find_projects(source_dir))


def _read_json(path: str) -> dict[str, Any] | None:
    """Reads a JSON file, tolerating it being absent or malformed.

    Args:
        path: Path to the JSON file.

    Returns:
        The parsed object, or None if it couldn't be read/parsed.
    """
    try:
        with open(path) as f:
            # json.load returns Any; a tsconfig is always a JSON object.
            return cast(dict[str, Any], json.load(f))
    except (OSError, json.JSONDecodeError):
        return None


def _resolve_entry_config(project_dir: str) -> tuple[str, list[str]]:
    """Picks the tsconfig and entry points typedoc should actually use.

    A root tsconfig.json with its own `include`/`files` is used directly;
    one that only `references` child configs (common with Vite's app/node
    split) is followed one level to find a config that lists real files.
    Falls back to documenting the whole project directory if neither is
    found, so a non-standard layout still gets *something* generated.

    Args:
        project_dir: Directory containing the project's tsconfig.json.

    Returns:
        A (tsconfig_path, entry_points) pair to pass to typedoc.
    """
    root_config_path = os.path.join(project_dir, "tsconfig.json")
    cfg = _read_json(root_config_path) or {}

    include = cfg.get("include") or cfg.get("files") or []
    if include:
        return root_config_path, include

    for ref in cfg.get("references") or []:
        ref_path = ref.get("path")
        if not ref_path:
            continue
        ref_config_path = os.path.normpath(os.path.join(project_dir, ref_path))
        if os.path.isdir(ref_config_path):
            ref_config_path = os.path.join(ref_config_path, "tsconfig.json")
        ref_cfg = _read_json(ref_config_path)
        if ref_cfg is None:
            continue
        ref_include = ref_cfg.get("include") or ref_cfg.get("files")
        if ref_include:
            return ref_config_path, ref_include

    return root_config_path, ["."]


def extract(source_dir: str, out_dir: str) -> ExtractResult:
    """Documents every TypeScript project under `source_dir`.

    Args:
        source_dir: Root of the workspace to search for tsconfig.json files.
        out_dir: Directory to write each project's generated HTML into,
            nested under a subdirectory per project.

    Returns:
        The combined result across all projects found.
    """
    projects = find_projects(source_dir)
    if not projects:
        return {"ok": True, "detail": "no tsconfig.json found, nothing to do", "targets": []}

    targets: list[Target] = []
    for project_dir in projects:
        project_name = os.path.relpath(project_dir, source_dir)
        tsconfig_path, entry_points = _resolve_entry_config(project_dir)
        project_out_dir = os.path.join(out_dir, project_name)
        os.makedirs(project_out_dir, exist_ok=True)

        try:
            result = subprocess.run(
                [
                    "typedoc",
                    "--tsconfig",
                    tsconfig_path,
                    "--out",
                    project_out_dir,
                    "--entryPointStrategy",
                    "expand",
                    "--skipErrorChecking",
                    "--exclude",
                    "**/node_modules/**",
                    *entry_points,
                ],
                cwd=project_dir,
                capture_output=True,
                text=True,
                timeout=600,
            )
        except (OSError, subprocess.TimeoutExpired) as e:
            targets.append({"project": project_name, "ok": False, "detail": f"typedoc failed: {e}"})
            continue

        ok = result.returncode == 0
        detail = "generated" if ok else (result.stderr.strip()[-500:] or "typedoc failed")
        targets.append(
            {"project": project_name, "ok": ok, "detail": detail, "out_dir": project_out_dir}
        )

    ok = all(t["ok"] for t in targets)
    return {"ok": ok, "detail": f"{len(targets)} project(s) documented", "targets": targets}
