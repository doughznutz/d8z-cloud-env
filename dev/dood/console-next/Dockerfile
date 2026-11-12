# console-next - the TypeScript front door for one dood gateway: terminates
# multi-user auth, owns the service-API-key store, and talks to Go's
# gw-server (docker/job execution backend) and its embedded NATS server
# directly. Ships as its own sibling container (docker-compose.yaml),
# fronted by the same root Traefik as everything else (dynamic/base.yml).
#
# Build context is the repo root (see docker-compose.yaml), matching
# dev/dood/Dockerfile's convention, so this stage can reuse gw-server's
# generated types.gen.ts without duplicating the schemagen step.

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY dev/dood/server/go.mod dev/dood/server/go.sum ./
RUN go mod download
COPY dev/dood/server/ ./
RUN go run ./cmd/schemagen -out-schema ./schema -out-ts /src/types.gen.ts

FROM node:22-alpine AS build
WORKDIR /src
COPY dev/dood/console-next/package.json dev/dood/console-next/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci --no-audit --loglevel=error; else npm install --no-audit --loglevel=error; fi
COPY dev/dood/console-next/ ./
# The TypeScript derived from gw-server's own structs (server stage, above) -
# the same file dev/dood/console's own build stage copies in.
COPY --from=server /src/types.gen.ts ./src/types.gen.ts
# Fail the build on any type error, lint error, or missing JSDoc comment on
# exported code (rules: tsconfig*.json, eslint.config.js).
RUN npm run typecheck
RUN npm run lint
RUN npm run build

FROM node:22-alpine
RUN apk add --no-cache dumb-init
WORKDIR /app
ENV NODE_ENV=production
COPY dev/dood/console-next/package.json dev/dood/console-next/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci --omit=dev --no-audit --loglevel=error; else npm install --omit=dev --no-audit --loglevel=error; fi
COPY --from=build /src/dist/ ./dist/
EXPOSE 8787
ENTRYPOINT ["dumb-init", "node", "dist/server.js"]
