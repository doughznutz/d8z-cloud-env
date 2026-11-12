"""Lists the containers on this host's Docker daemon, for the Targets tab.

Read-only in use, but the socket it talks through is not: anything that
can reach /var/run/docker.sock can drive the whole daemon. That's why only
gopher's dashboard container mounts it - gopher-agents, where the LLM CLIs
run, never does.
"""

from typing import TypedDict

import docker


class Target(TypedDict):
    """One container as shown in the Targets tab."""

    name: str
    image: str
    status: str
    ports: str


def list_targets() -> list[Target]:
    """Lists every container (running or not) on the Docker daemon.

    Returns:
        One row per container, by name.

    Raises:
        docker.errors.DockerException: If the daemon can't be reached.
    """
    client = docker.from_env()
    try:
        rows: list[Target] = []
        for c in client.containers.list(all=True):
            ports = (c.attrs.get("NetworkSettings") or {}).get("Ports") or {}
            image = c.image.tags[0] if c.image and c.image.tags else (c.attrs.get("Image") or "")
            rows.append(
                {
                    "name": c.name or "",
                    "image": image[:40],
                    "status": c.status or "",
                    "ports": ", ".join(sorted(ports)) or "—",
                }
            )
        return sorted(rows, key=lambda r: r["name"])
    finally:
        client.close()
