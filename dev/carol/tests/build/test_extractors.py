"""The generated docs describe the code without embedding its source.

Agents read these pages (as text), so any source in them is paid for in
tokens - and source isn't what the docs are for. Each test plants a marker
inside a function body and checks it never reaches the output.
"""

from pathlib import Path

import pytest

from server.docserver import render_text
from server.extractors import python, shell

#: Appears only inside function bodies in the fixtures below.
BODY = "BODY_MARKER_7f3a"


@pytest.fixture
def source(tmp_path: Path) -> Path:
    """A tiny workspace: one Python package, one standalone script, one shell script.

    Args:
        tmp_path: pytest's per-test temporary directory.

    Returns:
        The workspace root.
    """
    root = tmp_path / "src"
    pkg = root / "pkg"
    pkg.mkdir(parents=True)
    (pkg / "__init__.py").write_text(
        f'"""A package."""\n\n\ndef api() -> str:\n    """Documented API."""\n    return "{BODY}"\n'
    )
    (root / "app.py").write_text(
        f'"""A script."""\n\n\ndef main() -> None:\n    """Entry point."""\n    print("{BODY}")\n'
    )
    (root / "run.sh").write_text(
        f'#!/bin/sh\n# Starts things.\n\n# Says hello.\nhello() {{\n  echo "{BODY}"\n}}\n'
    )
    return root


def _all_output(out_dir: Path) -> str:
    """Concatenates every generated file under a directory.

    Args:
        out_dir: Extractor output directory.

    Returns:
        All their text.
    """
    return "".join(p.read_text(errors="replace") for p in out_dir.rglob("*") if p.is_file())


def _visible_text(out_dir: Path) -> str:
    """Renders every generated page as the text view agents (and people) see.

    Args:
        out_dir: Extractor output directory.

    Returns:
        All pages' visible text.
    """
    return "".join(
        render_text(p.read_text(errors="replace"), "python", "/" + p.name)
        for p in out_dir.rglob("*.html")
    )


def test_python_docs_have_no_source(source: Path, tmp_path: Path) -> None:
    """Both pdoc (packages) and the script reader document APIs, never bodies."""
    out = tmp_path / "out"
    result = python.extract(str(source), str(out))
    assert result["ok"], result
    text = _all_output(out)
    assert "Documented API." in text and "Entry point." in text  # docstrings: yes
    assert BODY not in text  # bodies: nowhere in any generated file
    # pdoc's embedded-source block is off. (Its stylesheet still names a
    # "View Source" button, so check what's visible, not the raw files.)
    assert "View Source" not in _visible_text(out)


def test_shell_docs_have_no_source(source: Path, tmp_path: Path) -> None:
    """The shell reader lists a script's comments and functions, never its code."""
    out = tmp_path / "out"
    result = shell.extract(str(source), str(out))
    assert result["ok"], result
    text = _all_output(out)
    assert "Starts things." in text and "hello()" in text and "Says hello." in text
    assert BODY not in text
