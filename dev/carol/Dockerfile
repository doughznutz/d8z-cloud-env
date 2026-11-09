# ==============================================================================
# Stage 1: base runtime - the toolchains every extractor shells out to.
# ==============================================================================
FROM python:3.12-slim AS base

ENV DEBIAN_FRONTEND=noninteractive

# Go (go doc) and Node.js (typedoc) toolchains alongside this image's own
# Python, so each extractor can shell out to the real doc tool for its
# language instead of reimplementing doc parsing. curl: the report agents'
# one way to read the docs (server/docserver.py's text view).
RUN apt-get update && apt-get install -y --no-install-recommends \
    golang-go \
    nodejs \
    npm \
    git \
    ripgrep \
    curl \
    && rm -rf /var/lib/apt/lists/*

# nats-py: the client for dood's embedded NATS server (server/bus.py).
# pyyaml: ~/.d8z/carol.yaml, carol's settings file (server/settings.py).
# pytest: tests/ - build-time here, and post-deploy via tests/run.sh.
RUN pip install --no-cache-dir streamlit pdoc nats-py pyyaml pytest
RUN npm install -g typedoc typescript

# Go needs a writable cache/module path; keep it out of the read-only
# source mount and the docs volume alike.
ENV GOCACHE=/tmp/go-cache \
    GOPATH=/tmp/go-path

# ==============================================================================
# Stage 2: lint - enforces formatting, lint rules (incl. docstrings-present)
# and strict typing on the actual image source. This stage's own tooling
# (ruff/black/mypy) never reaches the final image; only its *verified*
# /app directory does, via the COPY --from below. A failure here fails the
# whole `docker build`.
# ==============================================================================
FROM base AS lint

RUN pip install --no-cache-dir ruff black mypy types-PyYAML

WORKDIR /app
COPY pyproject.toml /app/pyproject.toml
COPY app.py /app/app.py
COPY server/ /app/server/
COPY tests/ /app/tests/

RUN black --check .
RUN ruff check .
RUN mypy app.py server tests

# Just the static binary - carol's own toolchains (go/node/python) still
# come from `base`, so this never becomes the image's base like it is for
# dood (dev/dood/Dockerfile), which has no such toolchains to keep.
FROM traefik:v3.1 AS traefik_bin

# ==============================================================================
# Stage 3: the runtime image, before its tests have run.
# ==============================================================================
FROM base AS runtime

# LLM agent CLIs for the Reports tab (server/agents.py) - the same four
# deightz ships (dev/deightz/Dockerfile). Final stage only: the lint stage
# never runs them. They only ever run in the carol-agents container (same
# image, see docker-compose.yaml), whose API keys arrive at runtime, never
# baked in.
RUN npm install -g \
    @anthropic-ai/claude-code \
    @google/gemini-cli \
    @openai/codex \
    @continuedev/cli

# Each agent's own limits (agent-config/): read the docs only as plain text
# from carol's doc server with curl - no file tools, no other commands.
# Claude's and gemini's are system-level files a run can't override. cn's
# model config isn't baked in: server/agents.py writes it from carol.yaml.
COPY agent-config/claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY agent-config/gemini-policy.toml /etc/gemini-cli/policies/carol.toml
# The CLIs themselves run as this unprivileged user (server/agents.py), so
# the OS - not just their configs - keeps them out of everything that isn't
# world-readable, including /app (made root-only below).
RUN useradd --system --uid 10001 --home-dir /var/lib/carol-agent --create-home \
      --shell /usr/sbin/nologin carol-agent
COPY agent-config/continue-permissions.yaml /var/lib/carol-agent/.continue/permissions.yaml
COPY agent-config/codex-config.toml /var/lib/carol-agent/.codex/config.toml
RUN mkdir -p /var/lib/carol-agent/work && chown -R carol-agent:carol-agent /var/lib/carol-agent

WORKDIR /app
# Copied from the lint stage (not straight from the build context) so this
# image can only be built from source that already passed the checks above.
COPY --from=lint /app /app
# Carol's own code: root-only, so the agent CLIs (carol-agent) can't read it.
RUN chmod 700 /app
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Traefik fronts everything on this one port: the Streamlit dashboard and
# a static server per language's generated doc tree (see
# traefik/dynamic/base.yml) - all behind the same port this image has
# always exposed, so dood's own routing (dev/dood/dynamic/base.yml, which
# points at http://carol:8503 unconditionally) needs no changes.
COPY --from=traefik_bin /usr/local/bin/traefik /usr/local/bin/traefik
COPY traefik/ /etc/traefik/
EXPOSE 8503

ENTRYPOINT ["/app/entrypoint.sh"]

# ==============================================================================
# Stage 4: tests/build against the runtime image itself - the agent configs
# where the CLIs will load them, the agent user, /app's permissions, the
# extractors and the doc server's text view (tests/README.md). No keys, no
# network. A failure here fails the whole `docker build`.
# ==============================================================================
FROM runtime AS test
RUN cd /app && python3 -m pytest -q -p no:cacheprovider tests/build \
    && touch /tmp/build-tests.passed

# ==============================================================================
# Stage 5: the final image - the runtime stage, but only buildable once its
# tests have passed (the COPY makes this stage depend on the test stage, the
# same trick the lint stage uses for /app).
# ==============================================================================
FROM runtime
COPY --from=test /tmp/build-tests.passed /tmp/build-tests.passed
