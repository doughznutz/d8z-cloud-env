"""Extracts Go API docs with the standard `go doc` tool.

No extra dependency beyond the Go toolchain itself. A workspace can hold
several modules (this one does: dev/dood/server is its own go.mod deep
inside the tree), so every go.mod under source_dir is treated as its own
module root and documented independently, with `go doc -all <pkg>`'s
output written to its own text file per package.
"""

import os
import subprocess

from ..models import ExtractResult, Target

#: Directories that are never a module's own source, so walking into them
#: would waste time (and, for node_modules/vendor, duplicate vendored copies
#: of packages we'd already document from their real home).
SKIP_DIRS = {".git", "node_modules", "vendor"}


def find_modules(source_dir: str) -> list[str]:
    """Finds every directory under `source_dir` that contains a go.mod file.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        Absolute paths of each module root found, in walk order.
    """
    modules: list[str] = []
    for root, dirs, files in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        if "go.mod" in files:
            modules.append(root)
    return modules


def detect(source_dir: str) -> bool:
    """Reports whether `source_dir` contains any Go modules at all.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        True if at least one go.mod file was found.
    """
    return bool(find_modules(source_dir))


def extract(source_dir: str, out_dir: str) -> ExtractResult:
    """Documents every package in every Go module under `source_dir`.

    Args:
        source_dir: Root of the workspace to search for go.mod files.
        out_dir: Directory to write one `<package>.txt` file per package into,
            nested under a subdirectory per module.

    Returns:
        The combined result across all modules and packages found.
    """
    modules = find_modules(source_dir)
    if not modules:
        return {"ok": True, "detail": "no go.mod found, nothing to do", "targets": []}

    targets: list[Target] = []
    for module_dir in modules:
        module_name = os.path.relpath(module_dir, source_dir)
        try:
            listed = subprocess.run(
                # -buildvcs=false: source_dir is a read-only bind mount, so
                # git's ownership check on it fails inside the container.
                ["go", "list", "-buildvcs=false", "./..."],
                cwd=module_dir,
                capture_output=True,
                text=True,
                timeout=180,
            )
        except (OSError, subprocess.TimeoutExpired) as e:
            targets.append({"module": module_name, "ok": False, "detail": str(e)})
            continue

        if listed.returncode != 0:
            detail = listed.stderr.strip() or "go list failed"
            targets.append({"module": module_name, "ok": False, "detail": detail[-500:]})
            continue

        module_out_dir = os.path.join(out_dir, module_name)
        os.makedirs(module_out_dir, exist_ok=True)
        for pkg in [p for p in listed.stdout.splitlines() if p.strip()]:
            doc = subprocess.run(
                ["go", "doc", "-all", pkg],
                cwd=module_dir,
                capture_output=True,
                text=True,
                timeout=60,
            )
            out_path = os.path.join(module_out_dir, pkg.replace("/", "_") + ".txt")
            with open(out_path, "w") as f:
                f.write(doc.stdout)
            targets.append(
                {"module": module_name, "package": pkg, "ok": doc.returncode == 0, "file": out_path}
            )

    ok = all(t["ok"] for t in targets) if targets else True
    return {
        "ok": ok,
        "detail": f"{len(modules)} module(s), {len(targets)} package(s)",
        "targets": targets,
    }
