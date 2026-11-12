// console-next's own relational state - users, sessions, workspace ACLs -
// kept in one SQLite file (node:sqlite, built in, no native-module
// dependency to compile), outside any repo, mirroring how Go's
// workspace.yaml/deployment.yaml live outside the checkout.

import { DatabaseSync } from "node:sqlite";
import { mkdirSync, chmodSync } from "node:fs";
import { dirname } from "node:path";
import { loadConfig } from "./config.js";

const cfg = loadConfig();
mkdirSync(dirname(cfg.dbFile), { recursive: true });

/** db is console-next's single SQLite connection, synchronous like better-sqlite3 (and like Go's own file-backed stores: no network, no async driver). */
export const db = new DatabaseSync(cfg.dbFile);

try {
  chmodSync(cfg.dbFile, 0o600);
} catch {
  // Best effort - e.g. not owned by us on some mounts; not fatal.
}

db.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
  );
  CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
  );
  CREATE TABLE IF NOT EXISTS workspace_acl (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_name TEXT NOT NULL,
    role TEXT NOT NULL,
    PRIMARY KEY (user_id, workspace_name)
  );
  CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT
  );
  CREATE TABLE IF NOT EXISTS credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    label TEXT NOT NULL,
    secret_encrypted TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
  );
`);

/** hasColumn reports whether table (always one of our own hardcoded table names, never user input) already has column - used to add columns to a table that pre-existing installs created without them. */
function hasColumn(table: string, column: string): boolean {
  const cols = db.prepare(`SELECT name FROM pragma_table_info('${table}')`).all() as { name: string }[];
  return cols.some((c) => c.name === column);
}

// Phase 5: per-user docker-mode policy (src/auth/dockerPolicy.ts). Added via
// ALTER TABLE, guarded by hasColumn, so installs that created `users`
// before this column existed still pick it up on next start.
if (!hasColumn("users", "docker_policy")) {
  db.exec("ALTER TABLE users ADD COLUMN docker_policy TEXT NOT NULL DEFAULT 'host'");
}
