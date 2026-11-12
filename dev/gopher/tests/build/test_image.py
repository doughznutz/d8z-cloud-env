"""The image's OS-level limits on the agent CLIs.

Agent CLIs run as an unprivileged user (server/agents.py AGENT_USER); these
check that user exists and can't read what isn't meant for it - so a CLI
whose own config fails to hold still can't get at gopher's code.
"""

import os
import pwd
import shutil
import stat
import subprocess

import pytest

from server import agents


def _as_agent(*argv: str) -> subprocess.CompletedProcess[str]:
    """Runs a command as the agent user, the way server/agents.py runs a CLI.

    Args:
        *argv: The command.

    Returns:
        Its completed process.
    """
    return subprocess.run(
        argv,
        user=agents.AGENT_USER,
        group=agents.AGENT_USER,
        extra_groups=[],
        capture_output=True,
        text=True,
        timeout=30,
    )


def test_agent_user_is_unprivileged() -> None:
    """The agent user exists, isn't root, and has no login shell."""
    u = pwd.getpwnam(agents.AGENT_USER)
    assert u.pw_uid != 0 and u.pw_gid != 0
    assert u.pw_shell.endswith("nologin")


def test_gopher_code_is_root_only() -> None:
    """/app (gopher's own code) is root-owned and mode 0700."""
    st = os.stat("/app")
    assert st.st_uid == 0 and stat.S_IMODE(st.st_mode) == 0o700


@pytest.mark.skipif(os.getuid() != 0, reason="switching users needs root")
@pytest.mark.parametrize("path", ["/app/server/config.py", "/root"])
def test_agent_user_cannot_read(path: str) -> None:
    """The agent user is denied gopher's code and root's home."""
    result = _as_agent("ls", "-la", path)
    assert result.returncode != 0 and "Permission denied" in result.stderr


def test_curl_is_installed_for_the_agent_user() -> None:
    """Curl - the agents' only way to read the docs - is on the agent user's PATH."""
    assert shutil.which("curl"), "curl missing: every agent's allowed command would fail"
    if os.getuid() == 0:
        assert _as_agent("curl", "--version").returncode == 0


def test_agent_workdir_is_empty_and_theirs() -> None:
    """Every CLI starts in an empty directory the agent user owns."""
    st = os.stat(agents.AGENT_WORKDIR)
    assert st.st_uid == pwd.getpwnam(agents.AGENT_USER).pw_uid
    assert os.listdir(agents.AGENT_WORKDIR) == []
