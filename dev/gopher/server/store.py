"""Atomic JSON persistence shared by everything gopher keeps under STATE_DIR.

Doc-run history, check definitions and check results all save the same
way: write a temp file beside the target, then ``os.replace`` it into place,
so a crash mid-write can never leave a truncated file behind.
"""

import json
import os


def load_json(path: str, default: object) -> object:
    """Loads JSON from ``path``, or returns ``default`` if it doesn't exist yet.

    Args:
        path: Absolute path to the JSON file.
        default: Value to return when the file is missing.

    Returns:
        The parsed JSON content, or ``default`` - callers cast it to the
        shape they saved.
    """
    if not os.path.exists(path):
        return default
    with open(path) as f:
        return json.load(f)


def save_json(path: str, data: object) -> None:
    """Writes ``data`` to ``path`` as JSON, atomically.

    Args:
        path: Absolute path to write to; its directory is created if needed.
        data: JSON-serializable value to persist.
    """
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2)
    os.replace(tmp, path)
