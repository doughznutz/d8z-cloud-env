"""Extracts Python API docs with pdoc.

pdoc reads docstrings straight from well-documented modules/packages and
renders static HTML, with no project-specific config needed (unlike
Sphinx). A workspace can hold several packages at different depths, so
this walks the whole tree for "package roots": directories with
__init__.py whose parent isn't itself part of a package (pdoc documents a
package's whole subtree on its own, so nested packages are covered by
their root and don't need listing separately).
"""

import os
import subprocess

from ..models import ExtractResult, Target

#: Directories to never walk into: VCS metadata, caches, and virtualenvs,
#: none of which are this project's own source.
SKIP_DIRS = {".git", "node_modules", "venv", ".venv", "__pycache__"}


def find_packages(source_dir: str) -> list[str]:
    """Finds every top-level Python package root under `source_dir`.

    Stops descending into a directory once it's identified as a package
    root, since pdoc documents that package's whole subtree on its own.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        Absolute paths of each package root found, in walk order.
    """
    packages: list[str] = []
    for root, dirs, files in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        if "__init__.py" in files:
            packages.append(root)
            dirs[:] = []  # this package's own subtree is covered by pdoc
    return packages


def detect(source_dir: str) -> bool:
    """Reports whether `source_dir` contains any Python packages at all.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        True if at least one package root (a directory with __init__.py)
        was found.
    """
    return bool(find_packages(source_dir))


def extract(source_dir: str, out_dir: str) -> ExtractResult:
    """Documents every top-level Python package under `source_dir`.

    Args:
        source_dir: Root of the workspace to search for package roots.
        out_dir: Directory to write each package's generated HTML into,
            nested under a subdirectory per package.

    Returns:
        The combined result across all packages found.
    """
    packages = find_packages(source_dir)
    if not packages:
        return {"ok": True, "detail": "no Python packages found, nothing to do", "targets": []}

    os.makedirs(out_dir, exist_ok=True)
    targets: list[Target] = []
    for package_dir in packages:
        package_name = os.path.relpath(package_dir, source_dir)
        parent_dir = os.path.dirname(package_dir)
        module_name = os.path.basename(package_dir)
        # pdoc imports the package by name, so its parent must be importable.
        env = {**os.environ, "PYTHONPATH": parent_dir}
        package_out_dir = os.path.join(out_dir, package_name)
        os.makedirs(package_out_dir, exist_ok=True)

        try:
            result = subprocess.run(
                ["pdoc", "-o", package_out_dir, module_name],
                cwd=parent_dir,
                capture_output=True,
                text=True,
                timeout=300,
                env=env,
            )
        except (OSError, subprocess.TimeoutExpired) as e:
            targets.append({"package": package_name, "ok": False, "detail": f"pdoc failed: {e}"})
            continue

        ok = result.returncode == 0
        detail = "generated" if ok else (result.stderr.strip()[-500:] or "pdoc failed")
        targets.append(
            {"package": package_name, "ok": ok, "detail": detail, "out_dir": package_out_dir}
        )

    ok = all(t["ok"] for t in targets)
    return {"ok": ok, "detail": f"{len(targets)} package(s) documented", "targets": targets}
