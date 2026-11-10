# Dood - the D8Z gateway: Traefik + gw-server (Go) serving the React/Vite console and
# spawning child gateway bundles over the docker CLI.
#
# Build context is the repo root (see docker-compose.yaml): the whole repo
# is copied to /self so child bundles can be built and started from the
# same compose definitions as the root.

# Every Dockerfile in the repo passes hadolint (rules: /.hadolint.yaml)
# before this image - which carries the repo in /self, the source every
# optional service and child bundle is built from - can be built. Found by
# name, so a new service's Dockerfile is linted without being listed here.
FROM hadolint/hadolint:v2.12.0-alpine AS dockerfile-lint
COPY .hadolint.yaml /lint/.hadolint.yaml
COPY dev/ /lint/dev/
RUN find /lint/dev -name Dockerfile -print \
      -exec hadolint --no-color --config /lint/.hadolint.yaml {} + \
 && touch /tmp/dockerfiles.linted

FROM golang:1.26-alpine AS server
# golangci-lint, built with this stage's Go so it can load our go.mod's
# version. Installed before the source is copied, so the layer is cached.
ARG GOLANGCI_LINT_VERSION=v2.6.2
RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}
WORKDIR /src
COPY dev/dood/server/go.mod dev/dood/server/go.sum ./
RUN go mod download
COPY dev/dood/server/ ./
# JSON Schema and TypeScript, derived from the structs that define our JSON
# wire format (cmd/schemagen): schema/*.json lands in time for go build's
# //go:embed; types.gen.ts is picked up by the console stage below.
RUN go run ./cmd/schemagen -out-schema ./schema -out-ts /src/types.gen.ts
# Fail the build on any undocumented declaration (rules: server/.golangci.yml),
# a doc-comment problem golangci-lint can't see (cmd/doclint), or an
# unformatted file.
RUN test -z "$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
RUN golangci-lint run ./...
RUN go run ./cmd/doclint
# Fail the build on any failing test.
RUN CGO_ENABLED=0 go test ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gw-server .

FROM node:22-alpine AS console
WORKDIR /src
COPY dev/dood/console/package.json dev/dood/console/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci --no-audit --loglevel=error; else npm install --no-audit --loglevel=error; fi
COPY dev/dood/console/ ./
# The TypeScript derived from gw-server's own structs (server stage, above).
COPY --from=server /src/types.gen.ts ./src/types.gen.ts
# Fail the build on any type error, lint error, or missing JSDoc comment on
# exported code (rules: console/tsconfig*.json, console/eslint.config.js).
RUN npm run typecheck
RUN npm run lint
RUN npm run build

FROM traefik:v3.1
RUN apk add --no-cache openssl docker-cli docker-cli-compose docker-cli-buildx git

COPY --from=server /gw-server /usr/local/bin/gw-server
COPY --from=console /src/dist/ /srv/console/
COPY dev/dood/dynamic/ /etc/traefik/dynamic/
COPY dev/dood/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Depend on the dockerfile-lint stage, so this image only builds once it passed.
COPY --from=dockerfile-lint /tmp/dockerfiles.linted /tmp/dockerfiles.linted
COPY . /self
RUN find /self -name '*.sh' -exec sed -i 's/\r$//' {} + \
 && find /self -name '*.sh' -exec chmod +x {} +

ENTRYPOINT ["/entrypoint.sh"]
