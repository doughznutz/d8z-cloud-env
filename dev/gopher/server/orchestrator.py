"""Runs the per-language extractors and records their history.

This is the one place that knows how to "run an extractor and persist the
result" - today a Streamlit button click calls it directly; a future NATS
job handler can call the same functions without any UI code in between.
"""

import os
from datetime import datetime, timezone
from typing import cast

from . import config, settings, store
from .extractors import REGISTRY
from .models import History, RunRecord


def load_history() -> History:
    """Loads this server's persisted run history (one entry per language run).

    Returns:
        The persisted history, or {} if nothing has run yet.
    """
    return cast(History, store.load_json(config.STATE_FILE, {}))


def _record(history: History, language: str, result: RunRecord) -> RunRecord:
    """Appends one run's result to a language's history and persists it.

    Args:
        history: The full history to update in place.
        language: The language key the result belongs under.
        result: The result to append, with its timestamp already set.

    Returns:
        The same result that was recorded, for convenience.
    """
    hist = history.setdefault(language, [])
    hist.append(result)
    del hist[: -settings.load().docs_keep_runs]
    store.save_json(config.STATE_FILE, history)
    return result


def run_language(language: str, history: History | None = None) -> RunRecord:
    """Runs one language's extractor and records the result.

    Args:
        language: Key into `REGISTRY` (e.g. "go", "python").
        history: History to update; loaded from disk if not given. Passing
            an existing history in lets callers batch several runs (see
            `run_all`) without reloading/re-saving the whole file each time.

    Returns:
        The recorded result, with its timestamp set.

    Raises:
        ValueError: If `language` isn't a known extractor.
    """
    if language not in REGISTRY:
        raise ValueError(f"unknown language: {language}")
    history = history if history is not None else load_history()
    out_dir = os.path.join(config.DATA_DIR, language)
    result = REGISTRY[language].extract(config.SOURCE_DIR, out_dir)
    record: RunRecord = {**result, "ts": datetime.now(timezone.utc).isoformat(timespec="seconds")}
    return _record(history, language, record)


def run_all() -> dict[str, RunRecord]:
    """Runs every registered language's extractor and records each result.

    Returns:
        Each language's recorded result, keyed by language name.
    """
    history = load_history()
    return {language: run_language(language, history) for language in REGISTRY}
