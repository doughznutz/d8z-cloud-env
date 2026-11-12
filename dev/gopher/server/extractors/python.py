"""Extracts Python API docs with pdoc.

pdoc reads docstrings straight from well-documented modules/packages and
renders static HTML, with no project-specific config needed (unlike
Sphinx). A workspace can hold several packages at different depths, so
this walks the whole tree for "package roots": directories with
__init__.py whose parent isn't itself part of a package (pdoc documents a
package's whole subtree on its own, so nested packages are covered by
their root and don't need listing separately).

Standalone scripts - .py files outside every package, like each dev
container's Streamlit app.py - are documented too, but *not* with pdoc:
pdoc imports what it documents, and importing a Streamlit script runs the
whole dashboard (network calls, docker calls and all). Those are read with
`ast` instead, which never executes them.
"""

import ast
import html
import os
import subprocess

from ..models import ExtractResult, Target
from ._html import render_html_page, render_index_page, write

#: Directories to never walk into: VCS metadata, caches, and virtualenvs,
#: none of which are this project's own source.
SKIP_DIRS = {
    ".git",
    "node_modules",
    "venv",
    ".venv",
    "__pycache__",
    ".mypy_cache",
    ".ruff_cache",
    "dist",
    "build",
}


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


def find_scripts(source_dir: str) -> list[str]:
    """Finds every .py file under `source_dir` that isn't inside a package.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        Absolute paths of each standalone script, in walk order.
    """
    scripts: list[str] = []
    for root, dirs, files in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        if "__init__.py" in files:
            dirs[:] = []  # a package: pdoc's, not ours
            continue
        scripts.extend(os.path.join(root, f) for f in sorted(files) if f.endswith(".py"))
    return scripts


def detect(source_dir: str) -> bool:
    """Reports whether `source_dir` contains any Python packages or scripts at all.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        True if at least one package root or standalone script was found.
    """
    return bool(find_packages(source_dir) or find_scripts(source_dir))


def _leading_comment(source: str) -> str:
    """Returns a script's opening ``#`` comment block (after any shebang).

    Args:
        source: The script's source text.

    Returns:
        The comment text with ``#`` markers stripped, or "" if none.
    """
    lines: list[str] = []
    for line in source.splitlines():
        if line.startswith("#!") and not lines:
            continue
        if not line.startswith("#"):
            break
        lines.append(line.lstrip("#").strip())
    return "\n".join(lines).strip()


def _signature(node: ast.FunctionDef | ast.AsyncFunctionDef) -> str:
    """Renders a function's signature from its AST, as written in the source.

    Args:
        node: The function definition.

    Returns:
        e.g. ``async def fetch(url: str, timeout: int = 5) -> bytes``.
    """
    prefix = "async def" if isinstance(node, ast.AsyncFunctionDef) else "def"
    ret = f" -> {ast.unparse(node.returns)}" if node.returns else ""
    return f"{prefix} {node.name}({ast.unparse(node.args)}){ret}"


def _doc_html(node: ast.AST) -> str:
    """Renders a node's docstring as a preformatted block, or a placeholder.

    Args:
        node: A module, class or function node.

    Returns:
        HTML for the docstring.
    """
    if not isinstance(node, (ast.Module, ast.ClassDef, ast.FunctionDef, ast.AsyncFunctionDef)):
        return ""
    doc = ast.get_docstring(node)
    return f"<pre>{html.escape(doc)}</pre>" if doc else "<p><em>(no docstring)</em></p>"


def render_script(source: str, title: str) -> str:
    """Documents one script statically: docstrings, signatures, imports and constants.

    Args:
        source: The script's source text.
        title: Page heading (the script's path within the workspace).

    Returns:
        A complete HTML page.

    Raises:
        SyntaxError: If the script doesn't parse.
    """
    tree = ast.parse(source)
    parts: list[str] = []
    module_doc = ast.get_docstring(tree) or _leading_comment(source)
    parts.append(
        f"<pre>{html.escape(module_doc)}</pre>" if module_doc else "<p><em>(no docstring)</em></p>"
    )
    constants: list[str] = []
    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            parts.append(f"<h2><code>{html.escape(_signature(node))}</code></h2>{_doc_html(node)}")
        elif isinstance(node, ast.ClassDef):
            bases = ", ".join(ast.unparse(b) for b in node.bases)
            head = f"class {node.name}({bases})" if bases else f"class {node.name}"
            parts.append(f"<h2><code>{html.escape(head)}</code></h2>{_doc_html(node)}")
            for item in node.body:
                if isinstance(item, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    parts.append(
                        f"<h3><code>{html.escape(_signature(item))}</code></h3>{_doc_html(item)}"
                    )
        elif isinstance(node, (ast.Assign, ast.AnnAssign)):
            targets = node.targets if isinstance(node, ast.Assign) else [node.target]
            names = [t.id for t in targets if isinstance(t, ast.Name)]
            if names and all(n.isupper() for n in names):
                value = ast.unparse(node.value) if node.value else ""
                value = value if len(value) <= 120 else value[:117] + "..."
                constants.append(f"{' = '.join(names)} = {value}")
    if constants:
        parts.insert(1, "<h2>Constants</h2><pre>" + html.escape("\n".join(constants)) + "</pre>")
    imports = sorted(
        {
            (n.module or "") if isinstance(n, ast.ImportFrom) else a.name
            for n in ast.walk(tree)
            if isinstance(n, (ast.Import, ast.ImportFrom))
            for a in n.names
        }
        - {""}
    )
    if imports:
        parts.insert(1, "<h2>Imports</h2><pre>" + html.escape(", ".join(imports)) + "</pre>")
    return render_html_page(title, "".join(parts), back_href=_back_href(title))


def _back_href(rel_path: str) -> str:
    """Builds the relative link from a script's page back to the language index.

    Args:
        rel_path: The script's path within the workspace.

    Returns:
        e.g. "../../index.html" for "dev/og/app.py".
    """
    depth = rel_path.count("/")
    return "../" * depth + "index.html"


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
    scripts = find_scripts(source_dir)
    if not packages and not scripts:
        return {"ok": True, "detail": "no Python code found, nothing to do", "targets": []}

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
                # No embedded "View Source": the docs describe the API, and the
                # report agents read these pages - source would be most of
                # what they'd pay for.
                ["pdoc", "--no-show-source", "-o", package_out_dir, module_name],
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

    for script in scripts:
        rel = os.path.relpath(script, source_dir)
        page_path = os.path.join(out_dir, rel[: -len(".py")] + ".html")
        try:
            with open(script, encoding="utf-8", errors="replace") as f:
                write(page_path, render_script(f.read(), rel))
        except (OSError, SyntaxError, ValueError) as e:
            targets.append({"package": rel, "ok": False, "detail": f"couldn't read: {e}"})
            continue
        targets.append({"package": rel, "ok": True, "detail": "generated", "file": page_path})

    # One index over both kinds, since pdoc's per-package trees and the
    # script pages otherwise have no common entry point.
    links = [
        (f"{t['package']} (package)", f"{t['package']}/index.html")
        for t in targets
        if t["ok"] and "out_dir" in t
    ] + [
        (t["package"], t["package"][: -len(".py")] + ".html")
        for t in targets
        if t["ok"] and "file" in t
    ]
    write(os.path.join(out_dir, "index.html"), render_index_page("Python", None, links))

    ok = all(t["ok"] for t in targets)
    n_pkg = sum(1 for t in targets if not t["package"].endswith(".py"))
    detail = f"{n_pkg} package(s), {len(targets) - n_pkg} script(s) documented"
    return {"ok": ok, "detail": detail, "targets": targets}
