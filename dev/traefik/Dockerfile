# D8Z gateway: Traefik + gw-server (Go) serving the React/Vite console and
# spawning child gateway bundles over the docker CLI.
#
# Build context is the repo root (see docker-compose.yaml): the whole repo
# is copied to /self so child bundles can be built and started from the
# same compose definitions as the root.

FROM node:22-alpine AS console
WORKDIR /src
COPY dev/traefik/console/package.json dev/traefik/console/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci --no-audit --loglevel=error; else npm install --no-audit --loglevel=error; fi
COPY dev/traefik/console/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY dev/traefik/server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gw-server .

FROM traefik:v3.1
RUN apk add --no-cache openssl docker-cli docker-cli-compose docker-cli-buildx git

COPY --from=server /gw-server /usr/local/bin/gw-server
COPY --from=console /src/dist/ /srv/console/
COPY dev/traefik/dynamic/ /etc/traefik/dynamic/
COPY dev/traefik/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

COPY . /self
RUN find /self -name '*.sh' -exec sed -i 's/\r$//' {} + \
 && find /self -name '*.sh' -exec chmod +x {} +

ENTRYPOINT ["/entrypoint.sh"]
