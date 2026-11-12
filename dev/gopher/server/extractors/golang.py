"""Extracts Go API docs with the standard `go doc` tool.

No extra dependency beyond the Go toolchain itself. A workspace can hold
several modules (this one does: dev/dood/server is its own go.mod deep
inside the tree), so every go.mod under source_dir is treated as its own
module root and documented independently.

`go doc -all <pkg>`'s plain-text output is wrapped in a minimal static
HTML page per package - same relative-linked-static-tree shape as pdoc and
typedoc's own output (see python.py/typescript.py), rather than running a
live doc server: neither pkgsite nor godoc supports being mounted under a
URL subpath (both emit root-absolute links), which is exactly where this
ends up served from (see traefik/dynamic/base.yml).
"""

import os
import subprocess

from ..models import ExtractResult, Target
from ._html import render_doc_page as _render_doc_page
from ._html import render_index_page as _render_index_page

#: Directories that are never a module's own source, so walking into them
#: would waste time (and, for node_modules/vendor, duplicate vendored copies
#: of packages we'd already document from their real home).
SKIP_DIRS = {".git", "node_modules", "vendor"}


def _slug(pkg: str) -> str:
    """Turns an import path into a filesystem-safe, extensionless base name.

    Args:
        pkg: A package import path, e.g. "example.com/mod/sub".

    Returns:
        The import path with "/" replaced by "_".
    """
    return pkg.replace("/", "_")


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
        out_dir: Directory to write one `<package>.html` file per package
            into, nested under a subdirectory per module, plus an
            `index.html` per module and one more at `out_dir` itself.

    Returns:
        The combined result across all modules and packages found.
    """
    modules = find_modules(source_dir)
    if not modules:
        return {"ok": True, "detail": "no go.mod found, nothing to do", "targets": []}

    os.makedirs(out_dir, exist_ok=True)
    targets: list[Target] = []
    module_links: list[tuple[str, str]] = []

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
        package_links: list[tuple[str, str]] = []
        for pkg in [p for p in listed.stdout.splitlines() if p.strip()]:
            doc = subprocess.run(
                # -u: this workspace's Go code is internal tools, not
                # published libraries - most of the interesting logic
                # lives on unexported methods, which `go doc` otherwise
                # omits entirely.
                ["go", "doc", "-all", "-u", pkg],
                cwd=module_dir,
                capture_output=True,
                text=True,
                timeout=60,
            )
            page_name = _slug(pkg) + ".html"
            out_path = os.path.join(module_out_dir, page_name)
            with open(out_path, "w") as f:
                f.write(_render_doc_page(pkg, doc.stdout))
            package_links.append((pkg, page_name))
            targets.append(
                {"module": module_name, "package": pkg, "ok": doc.returncode == 0, "file": out_path}
            )

        # module_name can itself be several directories deep (e.g.
        # "dev/dood/server"), so the hop back to out_dir's own index.html
        # needs one "../" per component, not a single fixed "../".
        depth = len([s for s in os.path.normpath(module_name).split(os.sep) if s not in ("", ".")])
        back_href = "../" * depth + "index.html" if depth else "index.html"
        index_path = os.path.join(module_out_dir, "index.html")
        with open(index_path, "w") as f:
            f.write(_render_index_page(module_name, back_href, package_links))
        module_links.append((module_name, f"{module_name}/index.html"))

    with open(os.path.join(out_dir, "index.html"), "w") as f:
        f.write(_render_index_page("Go modules", None, module_links))

    ok = all(t["ok"] for t in targets) if targets else True
    return {
        "ok": ok,
        "detail": f"{len(modules)} module(s), {len(targets)} package(s)",
        "targets": targets,
    }
