# --- lint: static checks that must pass before the real image is built ---
# ruff enforces formatting, import order, and (via its "D"/pydocstyle rules)
# that every module/class/function carries a docstring - not just for style,
# but so a later doc-extraction tool has something to read. mypy checks the
# type hints. Nothing from this stage ships in the final image.
FROM python:3.12-slim AS lint

# Baseline (predates hadolint here, see /.hadolint.yaml): pin when next touched.
# hadolint ignore=DL3013
RUN pip install --no-cache-dir ruff mypy types-requests nats-py

WORKDIR /app
COPY pyproject.toml app.py ./

RUN ruff check . \
    && ruff format --check . \
    && mypy app.py

# --- runtime image ---
FROM python:3.12-slim

# Baseline (predates hadolint here, see /.hadolint.yaml): pin when next touched.
# hadolint ignore=DL3013
RUN pip install --no-cache-dir streamlit requests docker nats-py

WORKDIR /app
# Depending on the lint stage's own copy of app.py (rather than COPY-ing it
# again from the build context) means this image can't be built without
# that stage having passed first.
COPY --from=lint /app/app.py /app/app.py
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Streamlit dashboard.
EXPOSE 8502

ENTRYPOINT ["/app/entrypoint.sh"]
