"""Every extractor generates docs from the real workspace without failing.

Build-time tests (tests/build) use small fixture workspaces; this runs each
extractor against the workspace gopher actually documents, the way "Generate
all" does - so a change in the workspace that breaks generation (a tool's
config file it can't resolve, a syntax error an extractor chokes on) fails
here, not silently on the Docs tab. Output goes to a temporary directory:
the published docs are never touched.
"""

from pathlib import Path

import pytest

from server import config
from server.extractors import REGISTRY


@pytest.mark.parametrize("language", sorted(REGISTRY))
def test_extractor_succeeds_on_the_workspace(language: str, tmp_path: Path) -> None:
    """One language's extractor documents the real workspace cleanly."""
    extractor = REGISTRY[language]
    if not extractor.detect(config.SOURCE_DIR):
        pytest.skip(f"no {language} in {config.SOURCE_DIR}")
    result = extractor.extract(config.SOURCE_DIR, str(tmp_path / language))
    failed = [t for t in result["targets"] if not t["ok"]]
    assert result["ok"] and not failed, failed or result["detail"]
