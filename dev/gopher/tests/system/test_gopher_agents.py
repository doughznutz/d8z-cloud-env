"""The deployed gopher-agents container holds nothing but a way to the doc server.

Run inside gopher-agents, as root (tests/run.sh): these check the container
as compose actually started it - mounts, environment and network - which no
build-time test can see. Codex in particular has no per-command config, so
this container is the whole of its confinement.
"""

import os
import pwd
import re
import subprocess
import urllib.error
import urllib.request

import pytest

#: Mount points Docker gives every container; anything else was mounted in.
_DOCKER_MOUNTS = re.compile(r"^/$|^/(proc|dev|sys)(/|$)|^/etc/(hosts|hostname|resolv\.conf)$")

#: The only secrets gopher-agents is meant to receive: the agents' own keys.
_ALLOWED_SECRETS = {
    "ANTHROPIC_API_KEY",
    "CLAUDE_CODE_OAUTH_TOKEN",
    "GEMINI_API_KEY",
    "GOOGLE_API_KEY",
    "OPENAI_API_KEY",
    "CODEX_API_KEY",
}

#: Matches the secret-ish pattern but isn't a secret: the python base
#: image's public GPG fingerprint for verifying its own Python download.
_NOT_SECRETS = {"GPG_KEY"}

#: Each language's index, relative to the docs root.
_INDEXES = ["go/", "python/", "typescript/", "shell/"]


def test_nothing_is_mounted() -> None:
    """No volumes or bind mounts: no docs, no workspace, no gopher state or settings."""
    with open("/proc/self/mountinfo") as f:
        mounts = {line.split()[4] for line in f}
    extra = sorted(m for m in mounts if not _DOCKER_MOUNTS.match(m))
    assert not extra, f"unexpected mounts: {extra}"


def test_only_the_agents_keys_are_in_the_environment() -> None:
    """Of the .env's secrets, only the LLM keys arrive (e.g. no GITHUB_TOKEN)."""
    secretish = {k for k in os.environ if re.search(r"KEY|TOKEN|SECRET|PASSWORD", k)}
    secretish -= _NOT_SECRETS
    assert secretish <= _ALLOWED_SECRETS, sorted(secretish - _ALLOWED_SECRETS)


@pytest.fixture(scope="module")
def docs_url() -> str:
    """The doc server's base URL, as gopher gives it to agents.

    Returns:
        e.g. http://gopher:8502/gopher/docs/ - from GOPHER_DOCS_URL (run.sh sets it).
    """
    url = os.environ.get("GOPHER_DOCS_URL", "")
    if not url:
        pytest.skip("GOPHER_DOCS_URL not set (tests/run.sh sets it from gopher)")
    return url


def _text(url: str) -> str:
    """Fetches one text-view page, checking it really is the text view.

    Args:
        url: The page's URL (with ?format=text).

    Returns:
        The page text.
    """
    with urllib.request.urlopen(url, timeout=10) as r:
        assert r.headers.get("Content-Type", "").startswith("text/plain"), url
        return str(r.read().decode())


def test_agents_allowed_command_works(docs_url: str) -> None:
    """The exact command agents are allowed - curl -s '<url>' - works, as the agent user.

    This is what every agent actually runs (unlike the crawl below, which
    uses Python): if curl were missing or the doc server unreachable from
    here, every report would quietly have nothing to read.
    """
    from server import agents  # gopher-agents runs the same image

    user = agents.AGENT_USER if os.getuid() == 0 else None
    if user:
        pwd.getpwnam(user)
    result = subprocess.run(
        ["/bin/sh", "-c", f"curl -s '{docs_url}go/?format=text'"],
        user=user,
        group=user,
        extra_groups=[] if user else None,
        cwd=agents.AGENT_WORKDIR,
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, result.stderr
    assert "# Go modules" in result.stdout


def test_doc_server_text_view_is_complete(docs_url: str) -> None:
    """Every page is reachable from the language indexes: no broken links, no source."""
    seen: set[str] = set()
    queue = list(_INDEXES)
    broken: list[str] = []
    while queue:
        path = queue.pop().split("#")[0]
        if path in seen or path.startswith(("http:", "https:")):
            continue
        if path.split("/")[0] not in {i.rstrip("/") for i in _INDEXES}:
            continue  # bracketed text that merely looks like a link
        seen.add(path)
        try:
            text = _text(f"{docs_url}{path}?format=text")
        except urllib.error.URLError as e:
            broken.append(f"{path}: {e}")
            continue
        assert "View Source" not in text, f"embedded source in {path}"
        queue += re.findall(r"\]\(([^)\s#][^)\s]*)\)", text)
    assert not broken, broken
    for index in _INDEXES:
        assert index in seen
    assert len(seen) > len(_INDEXES), "indexes link to nothing - docs not generated?"
