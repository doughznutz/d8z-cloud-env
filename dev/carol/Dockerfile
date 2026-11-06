# ==============================================================================
# Stage 1: base runtime - the toolchains every extractor shells out to.
# ==============================================================================
FROM python:3.12-slim AS base

ENV DEBIAN_FRONTEND=noninteractive

# Go (go doc) and Node.js (typedoc) toolchains alongside this image's own
# Python, so each extractor can shell out to the real doc tool for its
# language instead of reimplementing doc parsing.
RUN apt-get update && apt-get install -y --no-install-recommends \
    golang-go \
    nodejs \
    npm \
    git \
    && rm -rf /var/lib/apt/lists/*

# nats-py: the client for dood's embedded NATS server (server/bus.py).
RUN pip install --no-cache-dir streamlit pdoc nats-py
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

RUN pip install --no-cache-dir ruff black mypy

WORKDIR /app
COPY pyproject.toml /app/pyproject.toml
COPY app.py /app/app.py
COPY server/ /app/server/

RUN black --check .
RUN ruff check .
RUN mypy app.py server

# Just the static binary - carol's own toolchains (go/node/python) still
# come from `base`, so this never becomes the image's base like it is for
# dood (dev/dood/Dockerfile), which has no such toolchains to keep.
FROM traefik:v3.1 AS traefik_bin

# ==============================================================================
# Stage 3: final runtime image.
# ==============================================================================
FROM base

WORKDIR /app
# Copied from the lint stage (not straight from the build context) so this
# image can only be built from source that already passed the checks above.
COPY --from=lint /app /app
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
