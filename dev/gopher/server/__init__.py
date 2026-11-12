"""Gopher's server package.

Everything behind the dashboard, kept free of Streamlit (or any transport)
so a future NATS job handler can drive it without pulling in UI code:

- checks/: user-defined HTTP/TCP checks against the cluster's containers
- orchestrator, extractors/, coverage, docserver: API docs generated from
  this workspace's source, and served as plain text to the report agents
- agents, agent_client, agent_runner, reports, prompts: one prompt sent to
  several LLM agent CLIs at once, run in the separate gopher-agents
- bus: the one module that talks to dood's NATS server
"""
