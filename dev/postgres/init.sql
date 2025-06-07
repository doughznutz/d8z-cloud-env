CREATE TABLE openai_logs (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id TEXT,
    model TEXT,
    request JSONB,
    response JSONB
);
