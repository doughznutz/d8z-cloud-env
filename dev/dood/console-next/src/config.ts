// Env-var loading, same getenv/getenvInt shape as server/config.go's
// loadConfig - one image, env-driven, no config file of its own.

import { dirname, join } from "node:path";

/** Config is console-next's view of itself and of the Go server it fronts. */
export interface Config {
  /** address this server's HTTP listener binds to */
  listen: string;
  /** base URL of the Go gw-server/Traefik entrypoint this console-next instance proxies to, as given (may carry embedded userinfo) */
  gwTarget: string;
  /** gwTarget with any embedded userinfo stripped - the actual fetch target (fetch() rejects URLs that carry credentials) */
  upstream: URL;
  /** Basic-auth header derived from gwTarget's embedded userinfo, if any, attached to every proxied request since fetch() won't do it for us */
  authHeader?: string;
  /** path to console-next's own SQLite file (users/sessions/workspace_acl) - outside any repo, mounted from a volume in prod, same convention as GW_WORKSPACE_FILE */
  dbFile: string;
  /** path to the symmetric key used to encrypt stored credentials at rest (src/crypto/secretBox.ts) - generated on first run if absent, 0600, separate from any Go key material */
  masterKeyFile: string;
  /** address of the Go gateway's plain NATS client port (server/nats.go's NatsClientPort) - unauthenticated by design on internal_net, dialed directly by src/natsClient.ts instead of through gwTarget's HTTP bridge */
  natsUrl: string;
  /** seeds a first admin user on startup if the users table is empty - unset in prod once that user exists */
  adminUser?: string;
  /** password for adminUser's one-time seeding */
  adminPassword?: string;
}

/** getenv returns the environment variable k, or def when it is unset or empty. */
function getenv(k: string, def: string): string {
  const v = process.env[k];
  return v && v !== "" ? v : def;
}

/** loadConfig builds the Config from CN_ and GW_ environment variables. */
export function loadConfig(): Config {
  // Same env var name/meaning as console/vite.config.ts's dev proxy target:
  // the Traefik entrypoint for a whole running dood stack (so /deightz,
  // /og, /dozzle and nested /gw/<name>/... paths resolve too, not just
  // gw-server's own :8000 listener).
  const gwTarget = getenv("GW_TARGET", "http://localhost:8080");
  const upstream = new URL(gwTarget);
  const authHeader = upstream.username
    ? `Basic ${Buffer.from(`${decodeURIComponent(upstream.username)}:${decodeURIComponent(upstream.password)}`).toString("base64")}`
    : undefined;
  upstream.username = "";
  upstream.password = "";
  const dbFile = getenv("CN_DB_FILE", "./console-next.db");
  return {
    listen: getenv("CN_LISTEN", "0.0.0.0:8787"),
    gwTarget,
    upstream,
    authHeader,
    dbFile,
    masterKeyFile: getenv("CN_MASTER_KEY_FILE", join(dirname(dbFile), ".console-next-master-key")),
    natsUrl: getenv("CN_NATS_URL", "localhost:4222"),
    adminUser: process.env.CN_ADMIN_USER,
    adminPassword: process.env.CN_ADMIN_PASSWORD,
  };
}
