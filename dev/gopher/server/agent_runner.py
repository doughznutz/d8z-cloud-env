"""The gopher-agents container's HTTP service: runs agent CLIs on gopher's behalf.

Started as ``python3 -m server.agent_runner`` (see docker-compose.yaml's
gopher-agents service), in a container whose only mount is the docs volume,
read-only. That isolation is the whole point: gopher itself has the
workspace source mounted, so agents run here instead, where there's nothing
but the generated docs to read. See server/agents.py.

Endpoints (JSON, internal network only - no auth, like dood's NATS client
port):
    GET  /agents?ollama_url=...   availability of every agent
    POST /run                     {agent, prompt, timeout, ollama_url, models}
                                  -> AgentOutcome, once the agent finishes
"""

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import cast
from urllib.parse import parse_qs, urlsplit

from . import agents

#: Port the service listens on, inside the container.
PORT = int(os.environ.get("GOPHER_AGENTS_PORT", "8519"))


class _Handler(BaseHTTPRequestHandler):
    """Serves the two endpoints; one thread per request (ThreadingHTTPServer)."""

    def _reply(self, code: int, body: object) -> None:
        """Sends a JSON response.

        Args:
            code: HTTP status.
            body: JSON-serialisable payload.
        """
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler's naming
        """Reports every agent's availability, given gopher's og URL."""
        url = urlsplit(self.path)
        if url.path != "/agents":
            self._reply(404, {"error": "not found"})
            return
        ollama_url = parse_qs(url.query).get("ollama_url", [""])[0]
        self._reply(
            200,
            {n: list(agents.availability(a, ollama_url)) for n, a in agents.AGENTS.items()},
        )

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler's naming
        """Runs one agent to completion and returns its outcome."""
        if self.path != "/run":
            self._reply(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            req = cast(dict[str, object], json.loads(self.rfile.read(length)))
            agent = agents.AGENTS[str(req["agent"])]
            prompt = str(req["prompt"])
            timeout = int(cast(int, req["timeout"]))
            ollama_url = str(req.get("ollama_url", ""))
            models = [str(m) for m in cast(list[object], req.get("models", []))]
        except (ValueError, KeyError, TypeError) as e:
            self._reply(400, {"error": f"bad request: {e}"})
            return
        self._reply(200, agents.run(agent, prompt, timeout, ollama_url, models))


def main() -> None:
    """Serves until the container stops."""
    ThreadingHTTPServer(("0.0.0.0", PORT), _Handler).serve_forever()


if __name__ == "__main__":
    main()
