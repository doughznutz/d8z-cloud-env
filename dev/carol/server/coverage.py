"""What carol documents of the workspace - and, as importantly, what it doesn't.

Every extractor finds its own targets by a project marker (go.mod,
tsconfig.json, __init__.py...), so a file outside any marker - or a
language with no extractor at all - would otherwise be skipped silently.
`scan` reports, per programming language, how many source files sit inside
something an extractor actually documents; `uninspected` lists every other
file in the workspace that no extractor reads (compose files, Dockerfiles,
dependency manifests, Go tests, config...), grouped by kind, so whatever
the docs - and the report agents reading them - can't see shows up on the
dashboard instead.
"""

import os
import re
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
    """One language's source files, and how many of them get documented.

    `reasons` groups the uncovered files by why they're not documented.
    """

    language: str
    extractor: str | None
    files: int
    covered: int
    uncovered: list[str]
    reasons: dict[str, list[str]]


class FileGroup(TypedDict):
    """Workspace files of one kind that no extractor reads."""

    kind: str
    files: list[str]


#: Kinds of file `uninspected` groups by, in display order: (kind, test on
#: the file's workspace-relative path). The first match wins; anything left
#: over is "Other".
_KINDS: list[tuple[str, "re.Pattern[str]"]] = [
    ("Compose files", re.compile(r"(^|/)(docker-)?compose[^/]*\.ya?ml$")),
    ("Dockerfiles", re.compile(r"(^|/)Dockerfile[^/]*$|\.dockerfile$")),
    (
        "Dependency manifests",
        re.compile(
            r"(^|/)(go\.mod|go\.sum|package(-lock)?\.json|yarn\.lock|pnpm-lock\.yaml"
            r"|pyproject\.toml|requirements[^/]*\.txt|Pipfile(\.lock)?)$"
        ),
    ),
    ("Go tests", re.compile(r"_test\.go$")),
    ("Other YAML (Traefik, config, CI)", re.compile(r"\.ya?ml$")),
    ("Markdown", re.compile(r"\.md$")),
    ("Other config", re.compile(r"\.(toml|json|conf|ini|cfg)$|(^|/)\.[^/]+$")),
]


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
    return _scan(source_dir)[0]


def _scan(source_dir: str) -> tuple[list[LanguageCoverage], set[str]]:
    """Does `scan`'s work, also returning which files it found documented.

    Args:
        source_dir: Root of the workspace.

    Returns:
        (scan's result, absolute paths of every documented source file).
    """
    go_roots = golang.find_modules(source_dir)
    ts_roots = typescript.find_projects(source_dir)
    shell_scripts = set(shell.find_scripts(source_dir))

    def why_not(language: str, path: str) -> str | None:
        """Says why a source file isn't documented, or None if it is."""
        if language == "python":
            return None  # packages via pdoc, everything else as scripts
        if language == "go":
            if path.endswith("_test.go"):
                return "test files - go doc doesn't document them"
            return None if _under_any(path, go_roots) else "outside any Go module (no go.mod)"
        if language in ("typescript", "javascript"):
            return None if _under_any(path, ts_roots) else "outside any tsconfig.json project"
        if language == "shell":
            return None
        return "no extractor for this language"

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
    documented: set[str] = set()
    for language, paths in files.items():
        reasons: dict[str, list[str]] = {}
        for p in paths:
            reason = why_not(language, p)
            if reason:
                reasons.setdefault(reason, []).append(os.path.relpath(p, source_dir))
            else:
                documented.add(p)
        missed = sorted(f for fs in reasons.values() for f in fs)
        out.append(
            {
                "language": language,
                "extractor": EXTRACTOR_FOR.get(language),
                "files": len(paths),
                "covered": len(paths) - len(missed),
                "uncovered": missed,
                "reasons": {r: sorted(fs) for r, fs in reasons.items()},
            }
        )
    return sorted(out, key=lambda c: -c["files"]), documented


def uninspected(source_dir: str) -> list[FileGroup]:
    """Lists every workspace file no extractor reads, grouped by kind.

    That's every file except the source files `scan` counts as documented:
    undocumented source (e.g. Go tests, a language with no extractor) plus
    everything that isn't source at all - compose files, Dockerfiles,
    manifests, config, docs. None of it reaches the generated docs, so
    neither a reader of them nor a report agent can see it.

    Args:
        source_dir: Root of the workspace.

    Returns:
        One group per kind present, in _KINDS order with "Other" last;
        files sorted within each.
    """
    documented = _scan(source_dir)[1]
    groups: dict[str, list[str]] = {}
    for root, dirs, names in os.walk(source_dir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in names:
            path = os.path.join(root, name)
            if path in documented:
                continue
            rel = os.path.relpath(path, source_dir)
            kind = next((k for k, pattern in _KINDS if pattern.search(rel)), None)
            if kind is None:
                language = EXTENSIONS.get(os.path.splitext(name)[1].lower())
                kind = "Source not documented" if language else "Other"
            groups.setdefault(kind, []).append(rel)
    order = [k for k, _ in _KINDS] + ["Source not documented", "Other"]
    return [{"kind": k, "files": sorted(groups[k])} for k in order if k in groups]
