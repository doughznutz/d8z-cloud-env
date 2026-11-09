"""Documents shell scripts by reading their own comments.

Shell has no standard doc tool, but scripts here follow the usual
convention: a comment block under the shebang saying what the script is
for, and a comment above each function. This picks those out (plus every
function's name). The source itself isn't included - the page says the
script exists and what it's for.

A file counts as a shell script if it ends in .sh/.bash, or has no
extension and a sh/bash/zsh shebang.
"""

import html
import os
import re

from ..models import ExtractResult, Target
from ._html import render_html_page, render_index_page, write

#: Directories to never walk into.
SKIP_DIRS = {".git", "node_modules", "vendor", "dist", "build", ".venv", "venv"}

#: Extensions that are always shell.
EXTENSIONS = (".sh", ".bash")

_SHEBANG = re.compile(rb"^#!\s*\S*(?:/|env\s+)(?:ba|z|da)?sh\b")
_FUNC = re.compile(r"^\s*(?:function\s+)?([A-Za-z_][A-Za-z0-9_:.-]*)\s*\(\s*\)\s*\{?")
_FUNC_KW = re.compile(r"^\s*function\s+([A-Za-z_][A-Za-z0-9_:.-]*)\s*\{?")


def _is_script(path: str) -> bool:
    """Reports whether a file is a shell script, by extension or shebang.

    Args:
        path: The file to check.

    Returns:
        True for .sh/.bash files and extensionless files with a shell shebang.
    """
    name = os.path.basename(path)
    if name.endswith(EXTENSIONS):
        return True
    if "." in name.lstrip("."):
        return False
    try:
        with open(path, "rb") as f:
            return bool(_SHEBANG.match(f.read(64)))
    except OSError:
        return False


def find_scripts(source_dir: str) -> list[str]:
    """Finds every shell script under `source_dir`.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        Absolute paths of each script, in walk order.
    """
    found: list[str] = []
    for root, dirs, files in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        found.extend(
            os.path.join(root, f) for f in sorted(files) if _is_script(os.path.join(root, f))
        )
    return found


def detect(source_dir: str) -> bool:
    """Reports whether `source_dir` contains any shell scripts.

    Args:
        source_dir: Root of the workspace to search.

    Returns:
        True if at least one script was found.
    """
    return bool(find_scripts(source_dir))


def _comment_block(lines: list[str]) -> str:
    """Strips ``#`` markers from a run of comment lines.

    Args:
        lines: Consecutive comment lines.

    Returns:
        Their text, one line each.
    """
    return "\n".join(line.strip().lstrip("#").strip() for line in lines).strip()


def render_script(source: str, title: str, back_href: str) -> str:
    """Documents one script: its header comment and its functions.

    Args:
        source: The script's text.
        title: Page heading (the script's path within the workspace).
        back_href: Relative link back to the language index.

    Returns:
        A complete HTML page.
    """
    lines = source.splitlines()
    header: list[str] = []
    for line in lines:
        if line.startswith("#!") and not header:
            continue
        if not line.strip().startswith("#"):
            break
        header.append(line)
    parts = [
        (
            f"<pre>{html.escape(_comment_block(header))}</pre>"
            if header
            else "<p><em>(no header comment)</em></p>"
        )
    ]

    pending: list[str] = []
    funcs: list[str] = []
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("#") and not stripped.startswith("#!"):
            pending.append(line)
            continue
        m = _FUNC.match(line) or _FUNC_KW.match(line)
        if m:
            doc = _comment_block(pending)
            body = f"<pre>{html.escape(doc)}</pre>" if doc else "<p><em>(no comment)</em></p>"
            funcs.append(f"<h3><code>{html.escape(m.group(1))}()</code></h3>{body}")
        pending = []
    if funcs:
        parts.append("<h2>Functions</h2>" + "".join(funcs))

    return render_html_page(title, "".join(parts), back_href=back_href)


def extract(source_dir: str, out_dir: str) -> ExtractResult:
    """Documents every shell script under `source_dir`.

    Args:
        source_dir: Root of the workspace to search.
        out_dir: Directory to write one page per script (mirroring its path)
            and an index into.

    Returns:
        The combined result across all scripts found.
    """
    scripts = find_scripts(source_dir)
    if not scripts:
        return {"ok": True, "detail": "no shell scripts found, nothing to do", "targets": []}

    targets: list[Target] = []
    links: list[tuple[str, str]] = []
    for script in scripts:
        rel = os.path.relpath(script, source_dir)
        href = rel + ".html"
        try:
            with open(script, encoding="utf-8", errors="replace") as f:
                page = render_script(f.read(), rel, "../" * rel.count("/") + "index.html")
            write(os.path.join(out_dir, href), page)
        except OSError as e:
            targets.append({"file": rel, "ok": False, "detail": f"couldn't read: {e}"})
            continue
        targets.append({"file": rel, "ok": True, "detail": "generated"})
        links.append((rel, href))

    write(os.path.join(out_dir, "index.html"), render_index_page("Shell scripts", None, links))
    ok = all(t["ok"] for t in targets)
    return {"ok": ok, "detail": f"{len(links)} script(s) documented", "targets": targets}
