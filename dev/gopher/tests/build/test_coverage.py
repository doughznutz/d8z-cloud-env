"""The coverage scan reports every file the docs - and so the agents - can't see.

Gopher's extractors each find their own targets, so anything none of them
reads would otherwise vanish without a trace; these pin down that the scan
(server/coverage.py) surfaces it instead.
"""

from pathlib import Path

import pytest

from server import coverage


@pytest.fixture
def workspace(tmp_path: Path) -> Path:
    """A workspace with documented source and a file of every uninspected kind.

    Args:
        tmp_path: pytest's per-test temporary directory.

    Returns:
        The workspace root.
    """
    files = {
        "svc/go.mod": "module example.com/svc\n\ngo 1.26\n",
        "svc/svc.go": "// Package svc does things.\npackage svc\n",
        "svc/svc_test.go": "package svc\n",
        "loose/main.go": "package main\n",
        "app.py": '"""An app."""\n',
        "run.sh": "#!/bin/sh\n# Runs it.\n",
        "bin/launch": "#!/bin/bash\n# Launches it.\n",
        "docker-compose.yaml": "services: {}\n",
        "svc/Dockerfile": "FROM scratch\n",
        "web/package.json": "{}\n",
        "traefik/dynamic.yml": "http: {}\n",
        "README.md": "# Readme\n",
        ".hadolint.yaml": "ignored: []\n",
        "lib.rs": "fn main() {}\n",
        "logo.png": "",
    }
    for rel, text in files.items():
        path = tmp_path / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
    return tmp_path


def test_go_tests_and_loose_go_are_reported_with_reasons(workspace: Path) -> None:
    """Go test files and Go outside any module are undocumented, each saying why."""
    go = next(c for c in coverage.scan(str(workspace)) if c["language"] == "go")
    assert go["covered"] == 1  # svc/svc.go
    reasons = go["reasons"]
    assert reasons["test files - go doc doesn't document them"] == ["svc/svc_test.go"]
    assert reasons["outside any Go module (no go.mod)"] == ["loose/main.go"]


def test_uninspected_lists_everything_no_extractor_reads(workspace: Path) -> None:
    """Every file not documented is listed under its kind; documented ones aren't."""
    groups = {g["kind"]: g["files"] for g in coverage.uninspected(str(workspace))}
    assert groups["Compose files"] == ["docker-compose.yaml"]
    assert groups["Dockerfiles"] == ["svc/Dockerfile"]
    assert groups["Dependency manifests"] == ["svc/go.mod", "web/package.json"]
    assert groups["Go tests"] == ["svc/svc_test.go"]
    assert groups["Other YAML (Traefik, config, CI)"] == [".hadolint.yaml", "traefik/dynamic.yml"]
    assert groups["Markdown"] == ["README.md"]
    assert groups["Source not documented"] == ["lib.rs", "loose/main.go"]
    assert groups["Other"] == ["logo.png"]
    listed = {f for files in groups.values() for f in files}
    for documented in ("svc/svc.go", "app.py", "run.sh", "bin/launch"):
        assert documented not in listed
