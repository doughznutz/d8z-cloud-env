"""Each agent CLI's installed config limits it to curl against carol's doc server.

These read the files where the image installs them (dev/carol/Dockerfile),
so they check what the CLIs will actually load - not just the sources in
agent-config/.
"""

import json
import re
import tomllib
from pathlib import Path

import yaml

from server import agents, settings

#: The only command any agent may run.
DOC_SERVER = "http://carol:8503/"

#: File/edit/web tools no config-limited agent may have, per CLI's names.
CLAUDE_DENIED = {"Read", "Glob", "Grep", "Edit", "Write", "MultiEdit", "WebFetch", "WebSearch"}
GEMINI_DENIED = {
    "read_file",
    "read_many_files",
    "list_directory",
    "glob",
    "grep_search",
    "write_file",
    "replace",
    "web_fetch",
    "google_web_search",
    "run_shell_command",
}
CN_EXCLUDED = {"Read", "List", "Search", "Write", "Edit", "MultiEdit", "Fetch"}


def _is_doc_server_curl(command: str) -> bool:
    """Reports whether a command (or command pattern) is curl to the doc server only.

    Args:
        command: e.g. ``curl -s 'http://carol:8503/:*``.

    Returns:
        True if it's ``curl -s`` followed (optionally quoted) by the doc server.
    """
    return any(command.startswith(f"curl -s {q}{DOC_SERVER}") for q in ("", "'", '"'))


def test_claude_managed_settings() -> None:
    """Claude: only managed rules apply; curl-to-doc-server is the only allow."""
    cfg = json.loads(Path("/etc/claude-code/managed-settings.json").read_text())
    perms = cfg["permissions"]
    assert cfg["allowManagedPermissionRulesOnly"] is True
    assert perms["disableBypassPermissionsMode"] == "disable"
    assert perms["allow"], "nothing allowed - claude couldn't read the docs at all"
    for rule in perms["allow"]:
        assert rule.startswith("Bash(") and _is_doc_server_curl(rule[len("Bash(") :]), rule
    assert CLAUDE_DENIED <= set(perms["deny"])


#: Commands an agent must be able to run: the doc server's text view.
GOOD = [
    "curl -s 'http://carol:8503/carol/docs/go/?format=text'",
    "curl -s 'http://carol:8503/gw/review/carol/docs/python/dev/og/app.html?format=text'",
    "curl -s http://carol:8503/carol/docs/shell/?format=text",
]

#: Commands it must not: other files, other hosts, anything chained on.
BAD = [
    "cat /etc/passwd",
    "curl -s http://dood:80/",
    "curl -s 'http://carol:8503.evil.example/x'",
    "curl -s 'http://carol:8503/carol/docs/go/?format=text' ; cat /etc/passwd",
    "curl -s 'http://carol:8503/carol/docs/go/?format=text' && cat /etc/passwd",
    "curl -s 'http://carol:8503/carol/docs/go/?format=text' | sh",
    "curl -s 'http://carol:8503/$(cat /etc/passwd)'",
    "curl -s 'http://carol:8503/x' -o /tmp/out",
]


def _gemini_allows(rule: dict[str, object], command: str) -> bool:
    """Applies one gemini commandRegex rule the way gemini's policy engine does.

    Gemini (buildArgsPatterns) turns commandRegex into ``"command":"<regex>``
    and searches the tool call's JSON-encoded arguments with it.

    Args:
        rule: A [[rule]] table from the policy file.
        command: A shell command.

    Returns:
        True if the rule matches the command.
    """
    pattern = '"command":"' + str(rule["commandRegex"])
    # Compact separators: gemini encodes with JSON.stringify, which has none.
    encoded = json.dumps({"command": command}, separators=(",", ":"))
    return re.search(pattern, encoded) is not None


def test_gemini_policy() -> None:
    """Gemini: shell allowed only for curl-to-doc-server; every file/web tool denied."""
    rules = tomllib.loads(Path("/etc/gemini-cli/policies/carol.toml").read_text())["rule"]
    allows = [r for r in rules if r["decision"] == "allow"]
    denies = [r for r in rules if r["decision"] == "deny"]
    assert allows, "nothing allowed - gemini couldn't read the docs at all"
    for r in allows:
        assert r["toolName"] == "run_shell_command"
        # commandPrefix only ever matches at a word boundary in gemini, so a
        # prefix ending inside the URL would silently allow nothing.
        assert "commandPrefix" not in r and "commandRegex" in r
    for cmd in GOOD:
        assert any(_gemini_allows(r, cmd) for r in allows), f"denied: {cmd}"
    for cmd in BAD:
        assert not any(_gemini_allows(r, cmd) for r in allows), f"allowed: {cmd}"
    names = [r["toolName"] for r in denies]
    denied = {t for n in names for t in ([n] if isinstance(n, str) else n)}
    assert GEMINI_DENIED <= denied
    # The narrow allow must outrank the blanket shell deny, or curl is denied too.
    assert min(r["priority"] for r in allows) > max(r["priority"] for r in denies)


def _cn_allows(rule: str, command: str) -> bool:
    """Applies one cn ``Bash(<pattern>)`` rule the way cn does.

    cn turns the pattern into ``^...$`` with ``*`` as ".*" and ``?`` as ".",
    and matches it against the whole command line (it doesn't split
    compound commands).

    Args:
        rule: e.g. ``Bash(curl -s 'http://carol:8503/*)``.
        command: A shell command.

    Returns:
        True if the rule matches the command.
    """
    glob = rule[len("Bash(") : -1]
    pattern = re.sub(r"[.+^${}()|[\]\\]", lambda m: "\\" + m.group(0), glob)
    pattern = pattern.replace("*", ".*").replace("?", ".")
    return re.match(f"^{pattern}$", command) is not None


def test_continue_permissions() -> None:
    """Continue (cn): Bash allowed only for curl-to-doc-server; file/web tools excluded.

    cn's ``*`` matches anything, chained commands included, so its config
    alone can't stop ``curl ... ; cat x`` - like codex, cn is confined by the
    carol-agents container (nothing mounted, unprivileged user), which
    tests/system checks. This pins down that it *can* read the docs.
    """
    path = Path(agents.AGENT_HOME) / ".continue" / "permissions.yaml"
    cfg = yaml.safe_load(path.read_text())
    assert cfg["allow"], "nothing allowed - cn couldn't read the docs at all"
    for rule in cfg["allow"]:
        assert rule.startswith("Bash(") and _is_doc_server_curl(rule[len("Bash(") :]), rule
    for cmd in GOOD:
        assert any(_cn_allows(r, cmd) for r in cfg["allow"]), f"denied: {cmd}"
    for cmd in ("cat /etc/passwd", "curl -s http://dood:80/"):
        assert not any(_cn_allows(r, cmd) for r in cfg["allow"]), f"allowed: {cmd}"
    assert not cfg["ask"]
    assert CN_EXCLUDED <= set(cfg["exclude"])


def test_codex_config() -> None:
    """Codex: no approvals, keys kept out of its shell, its scratch dir trusted.

    Codex has no per-command rules - the carol-agents container (nothing
    mounted) is what confines it; tests/system checks that.
    """
    cfg = tomllib.loads((Path(agents.AGENT_HOME) / ".codex" / "config.toml").read_text())
    assert cfg["approval_policy"] == "never"
    env = cfg["shell_environment_policy"]
    assert env["inherit"] == "core" and {"*KEY*", "*TOKEN*"} <= set(env["exclude"])
    assert cfg["projects"][agents.AGENT_WORKDIR]["trust_level"] == "trusted"


def test_no_cli_flags_widen_the_configs() -> None:
    """No agent's command line grants tools or paths its config doesn't."""
    for agent in agents.AGENTS.values():
        argv = agent.argv("PROMPT")
        widening = ("--allowedTools", "--allow", "--auto", "--yolo")
        assert not any(a.startswith(widening) for a in argv)
        assert "--dangerously-skip-permissions" not in argv
        assert "/data" not in argv


def test_default_prompt_points_at_the_text_view() -> None:
    """The default context tells agents to curl ?format=text pages, not read files."""
    assert "{docs_url}" in settings.DEFAULT_CONTEXT
    assert "?format=text" in settings.DEFAULT_CONTEXT
    assert "curl -s '" in settings.DEFAULT_CONTEXT
