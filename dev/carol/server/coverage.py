"""Which programming languages the workspace uses, and how much carol documents.

Every extractor finds its own targets by a project marker (go.mod,
tsconfig.json, __init__.py...), so a file outside any marker - or a
language with no extractor at all - would otherwise be skipped silently.
This scans every source file by extension and reports, per language, how
many files sit inside something an extractor actually documents, so gaps
show up on the dashboard instead.
"""

import os
from typing import TypedDict

from .extractors import golang, shell, typescript

#: File extension -> language name. Anything not listed (config, markup,
#: data) isn't treated as source.
EXTENSIONS: dict[str, str] = {
    ".py": "python",
    ".go": "go",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".mts": "typescript",
    ".cts": "typescript",
    ".js": "javascript",
    ".jsx": "javascript",
    ".mjs": "javascript",
    ".cjs": "javascript",
    ".sh": "shell",
    ".bash": "shell",
    ".rs": "rust",
    ".java": "java",
    ".kt": "kotlin",
    ".rb": "ruby",
    ".php": "php",
    ".c": "c",
    ".h": "c",
    ".cc": "c++",
    ".cpp": "c++",
    ".hpp": "c++",
    ".cs": "c#",
    ".swift": "swift",
    ".lua": "lua",
    ".pl": "perl",
    ".scala": "scala",
    ".dart": "dart",
    ".ex": "elixir",
    ".exs": "elixir",
    ".sql": "sql",
}

#: Language -> the extractor (REGISTRY key) that documents it. JavaScript is
#: covered by typedoc only inside a tsconfig project.
EXTRACTOR_FOR: dict[str, str] = {
    "python": "python",
    "go": "go",
    "typescript": "typescript",
    "javascript": "typescript",
    "shell": "shell",
}

#: Directories that are never the workspace's own source.
SKIP_DIRS = {
    ".git",
    "node_modules",
    "vendor",
    "dist",
    "build",
    ".venv",
    "venv",
    "__pycache__",
    ".mypy_cache",
    ".ruff_cache",
}


class LanguageCoverage(TypedDict):
    """One language's source files, and how many of them get documented."""

    language: str
    extractor: str | None
    files: int
    covered: int
    uncovered: list[str]


def _under_any(path: str, roots: list[str]) -> bool:
    """Reports whether `path` is inside one of `roots`.

    Args:
        path: An absolute file path.
        roots: Absolute directory paths.

    Returns:
        True if any root is an ancestor of `path`.
    """
    return any(path.startswith(r.rstrip("/") + "/") for r in roots)


def scan(source_dir: str) -> list[LanguageCoverage]:
    """Counts source files per language and checks each against the extractors.

    Args:
        source_dir: Root of the workspace.

    Returns:
        One entry per language found, most files first.
    """
    go_roots = golang.find_modules(source_dir)
    ts_roots = typescript.find_projects(source_dir)
    shell_scripts = set(shell.find_scripts(source_dir))

    def covered(language: str, path: str) -> bool:
        if language == "python":
            return True  # packages via pdoc, everything else as scripts
        if language == "go":
            return _under_any(path, go_roots)
        if language in ("typescript", "javascript"):
            return _under_any(path, ts_roots)
        if language == "shell":
            return path in shell_scripts
        return False

    files: dict[str, list[str]] = {}
    for root, dirs, names in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in names:
            path = os.path.join(root, name)
            language = EXTENSIONS.get(os.path.splitext(name)[1].lower())
            if language is None and path in shell_scripts:
                language = "shell"  # extensionless, found by shebang
            if language:
                files.setdefault(language, []).append(path)

    out: list[LanguageCoverage] = []
    for language, paths in files.items():
        missed = [os.path.relpath(p, source_dir) for p in paths if not covered(language, p)]
        out.append(
            {
                "language": language,
                "extractor": EXTRACTOR_FOR.get(language),
                "files": len(paths),
                "covered": len(paths) - len(missed),
                "uncovered": sorted(missed),
            }
        )
    return sorted(out, key=lambda c: -c["files"])
