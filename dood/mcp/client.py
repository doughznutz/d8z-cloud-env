# client.py
# Minimal TCP JSON-line client for the MCP router (127.0.0.1:3456 inside the
# dood container). Python equivalent of callRouter() in cli/index.js, shared
# by the dashboard and the gateway.
import json
import socket

ROUTER_HOST = "127.0.0.1"
ROUTER_PORT = 3456


def call_router(request: dict, host: str = ROUTER_HOST, port: int = ROUTER_PORT, timeout: float = 30.0) -> dict:
    with socket.create_connection((host, port), timeout=timeout) as sock:
        sock.sendall((json.dumps(request) + "\n").encode("utf-8"))
        chunks = []
        while True:
            chunk = sock.recv(4096)
            if not chunk:
                break
            chunks.append(chunk)
            if chunk.endswith(b"\n"):
                break
    data = b"".join(chunks).decode("utf-8")
    if not data:
        raise ConnectionError("Empty response from MCP router")
    return json.loads(data)
