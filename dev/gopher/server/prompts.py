"""Prompt presets for report runs, stored in ~/.d8z/gopher.yaml (see settings.py)."""

from . import config, settings
from .models import Prompt


def with_context(text: str) -> str:
    """Prefixes a prompt with the configured context (where the docs are, etc.).

    Args:
        text: The user's prompt.

    Returns:
        The full prompt every agent receives.
    """
    context = (
        settings.load()
        .prompt_context.replace("{docs_url}", config.DOCS_URL)
        .replace("{data_dir}", config.DATA_DIR)
    )
    return f"{context}\n\nTask:\n{text}"


def all_prompts() -> list[Prompt]:
    """Returns every preset, in the order gopher.yaml lists them.

    Returns:
        The presets.
    """
    return list(settings.load().prompts)


def save(title: str, text: str) -> None:
    """Saves a preset, replacing one with the same title in place, else appending.

    Args:
        title: Preset name.
        text: Prompt text.
    """
    current = all_prompts()
    new: Prompt = {"title": title, "text": text}
    if any(p["title"] == title for p in current):
        current = [new if p["title"] == title else p for p in current]
    else:
        current.append(new)
    settings.update_prompts(current)


def delete(title: str) -> None:
    """Deletes a preset.

    Args:
        title: Preset name.
    """
    settings.update_prompts([p for p in all_prompts() if p["title"] != title])
