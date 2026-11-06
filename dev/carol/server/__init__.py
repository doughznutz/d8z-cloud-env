"""Carol's server package.

Orchestration for generating API docs from this workspace's Go, TypeScript
and Python source. Kept free of Streamlit (or any transport) so the same
orchestrator can be driven by the dashboard today and by a future NATS job
handler without pulling in UI code.
"""
