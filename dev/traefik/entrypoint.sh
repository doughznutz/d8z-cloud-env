#!/bin/sh
# One image, every level of the gateway cascade. See dynamic/base.yml for
# what GW_ROLE / GW_PREFIX change in routing.
set -e

export GW_ROLE="${GW_ROLE:-root}"
export GW_ID="${GW_ID:-root}"
export GW_PREFIX="${GW_PREFIX:-}"
if [ "$GW_ROLE" = root ]; then
  export GW_PORT="${GW_PORT:-80}"
else
  export GW_PORT="${GW_PORT:-8080}"
fi

if [ "$GW_ROLE" = root ]; then
  # Regenerated on every start from TRAEFIK_USER/TRAEFIK_PASSWORD so the
  # htpasswd hash never has to be committed to the checked-in compose file.
  if [ -z "$TRAEFIK_PASSWORD" ]; then
    echo "TRAEFIK_PASSWORD is required for the root gateway" >&2
    exit 1
  fi
  HASH=$(openssl passwd -apr1 "${TRAEFIK_PASSWORD}")
  printf '%s:%s\n' "${TRAEFIK_USER}" "${HASH}" > /etc/traefik/users
fi

# Static config: only the entrypoint port differs between levels.
cat > /etc/traefik/traefik.yml <<EOF
entryPoints:
  web:
    address: ":${GW_PORT}"
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
log:
  level: INFO
EOF

# Child bundles are brought up with `docker compose --env-file /self/.env`
# (interpolation + the services' env_file). .env is kept out of the image,
# so build it from our own environment, minus container plumbing and the
# gateway's identity (each child gets its own via the generated override).
if [ ! -f /self/.env ]; then
  env | grep -E '^[A-Za-z_][A-Za-z0-9_]*=' \
      | grep -vE "^(PATH|HOME|PWD|OLDPWD|SHELL|SHLVL|TERM|HOSTNAME|COMPOSE_PROJECT_NAME|COMPOSE_PROFILES|DOCKER_HOST|GW_[A-Z_]*|_)=" \
      | grep -v "'" \
      | sed -E "s/^([^=]+)=(.*)$/\1='\2'/" > /self/.env
fi

# Console + gateway API on localhost; Traefik fronts it (router `console`).
# Kept alive in a loop so a crash doesn't take the console down for good.
(while true; do /usr/local/bin/gw-server || true; sleep 2; done) &

[ "$1" = traefik ] && shift
exec traefik "$@"
