#!/bin/bash
# d8z cloud gateway bootstrap - the startup-script of a Container-Optimized
# OS VM created by gw-server (gcp.go). Runs as root on every boot.
#
# Instance metadata carries everything: d8z-env (the compose .env), d8z-image
# (our gateway image on Docker Hub), d8z-profile (root | workspace) and
# d8z-user (whose /home holds projects/ and the gateway config, .d8z/).
#
# The image carries the repo's docker-compose.yaml in /self, so the profile
# is brought up by running compose from inside that image against this VM's
# daemon - nothing but docker is needed on the host.
set -u
MD=http://metadata.google.internal/computeMetadata/v1/instance/attributes
md() { curl -sf -H 'Metadata-Flavor: Google' "$MD/$1"; }
log() { echo "d8z: $*"; }

U=$(md d8z-user || echo d8z)
H=/home/$U
IMAGE=$(md d8z-image)
PROFILE=$(md d8z-profile || echo root)

mkdir -p "$H/projects" "$H/.d8z"
chmod 700 "$H/.d8z"
md d8z-env > "$H/.env"
chmod 600 "$H/.env"
# Deightz runs as uid 1000 and edits under projects/.
chown 1000:1000 "$H/projects"

# COS drops inbound traffic by default; the GCE firewall rule (allow_from)
# is what actually limits who reaches these.
for p in 80 7422; do
  iptables -C INPUT -p tcp --dport "$p" -j ACCEPT 2>/dev/null || iptables -A INPUT -p tcp --dport "$p" -j ACCEPT
done

# COS keeps /root read-only: give the docker CLI a writable config dir.
export DOCKER_CONFIG=$H/.docker
mkdir -p "$DOCKER_CONFIG"

log "pulling $IMAGE"
until docker pull "$IMAGE"; do log "pull failed, retrying"; sleep 10; done

compose() {
  docker run --rm \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$H/.env:/self/.env:ro" \
    --entrypoint docker "$IMAGE" \
    compose --project-directory /self -p d8z --profile "$PROFILE" "$@"
}

log "bringing up profile $PROFILE"
compose pull --ignore-buildable --quiet || log "compose pull failed; using local images"
until compose up -d --remove-orphans; do log "compose up failed, retrying"; sleep 10; done
log "gateway up"
