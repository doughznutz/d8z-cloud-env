#!/bin/sh
set -e

# Regenerated on every start from TRAEFIK_USER/TRAEFIK_PASSWORD so the
# htpasswd hash never has to be committed to the checked-in compose file.
mkdir -p /etc/traefik
HASH=$(openssl passwd -apr1 "${TRAEFIK_PASSWORD}")
printf '%s:%s\n' "${TRAEFIK_USER}" "${HASH}" > /etc/traefik/users

# Console shell (sidebar + iframe canvas), localhost-only; Traefik fronts it
# on `/` behind console-auth (see dynamic.yml).
httpd -p 127.0.0.1:8000 -h /srv/console

exec traefik "$@"
