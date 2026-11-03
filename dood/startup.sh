#!/bin/bash
# Starts the three background services (MCP router, MCP gateway, Streamlit
# dashboard) that make up DOOD, brings up Traefik (the front door to DOOD
# itself and the dev services), then runs the interactive terminal CLI in
# the foreground so it owns the container's tty. On exit (or SIGTERM/SIGINT)
# the background services are torn down with it; Traefik is left running.
set -e

# Services declare `env_file: ./.env`, but .env is kept out of the image (see
# .dockerignore). Build one from the environment we were started with
# (`docker run --env-file <(env)`), minus host-shell plumbing that would
# break the containers it gets injected into.
if [ ! -f /self/.env ]; then
  echo "Generating /self/.env from the container environment..."
  env | grep -E '^[A-Za-z_][A-Za-z0-9_]*=' \
      | grep -vE "^(PATH|HOME|PWD|OLDPWD|SHELL|SHLVL|TERM|HOSTNAME|HOSTTYPE|LOGNAME|NAME|LANG|DISPLAY|WAYLAND_DISPLAY|PULSE_SERVER|MOTD_SHOWN|LS_COLORS|LESSOPEN|LESSCLOSE|JAVA_HOME|COMPOSE_PROJECT_NAME|COMPOSE_PROFILES|DOCKER_HOST|_|(WSL|XDG|NVM)[A-Z0-9_]*)=" \
      | grep -v "'" \
      | sed -E "s/^([^=]+)=(.*)$/\1='\2'/" > /self/.env
fi

echo "Starting MCP Router (internal, 127.0.0.1:3456)..."
/opt/venv_mcp/bin/python3 /app/mcp/router/router.py "$@" &
ROUTER_PID=$!

echo "Waiting for MCP Router..."
until (exec 3<>/dev/tcp/127.0.0.1/3456) 2>/dev/null; do sleep 0.5; done
exec 3>&-
echo "MCP Router is ready."

echo "Starting MCP Gateway (external, :8766)..."
/opt/venv_mcp/bin/python3 -m uvicorn main:app --app-dir /app/gateway --host 0.0.0.0 --port 8766 &
GATEWAY_PID=$!

echo "Starting Streamlit dashboard (:8501)..."
/opt/venv_mcp/bin/streamlit run /app/dashboard/app.py \
  --server.port 8501 \
  --server.address 0.0.0.0 \
  --server.headless true \
  --server.baseUrlPath dood &
DASHBOARD_PID=$!

trap 'kill -TERM "$ROUTER_PID" "$GATEWAY_PID" "$DASHBOARD_PID" 2>/dev/null' TERM INT

# Traefik is the gateway to DOOD itself: start it (and its deps) via make,
# then (host-socket mode) put this container on internal_net as `dood` so
# Traefik's http://dood:8501 route reaches the dashboard above. In dind mode
# (DOOD_HOST set by dood-dind) Traefik reaches us via the sidecar's netns
# instead. Best-effort, so the CLI still comes up if docker has trouble.
echo "Starting Traefik (http://localhost:8080)..."
if make -C /self up-traefik; then
  if [ "${DOOD_HOST:-dood}" = "dood" ]; then
    docker network connect --alias dood internal_net "$(hostname)" 2>/dev/null || true
  fi
  echo "Traefik is up; console at http://localhost:8080/"
else
  echo "WARNING: failed to start Traefik; the web console will be unavailable."
fi

echo "Starting interactive terminal client..."
node /app/cli/index.js "$@"
CLI_EXIT=$?

kill -TERM "$ROUTER_PID" "$GATEWAY_PID" "$DASHBOARD_PID" 2>/dev/null || true
exit "$CLI_EXIT"
