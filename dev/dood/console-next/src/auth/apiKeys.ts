// Service API keys: a user-provisioned bearer token for programmatic access
// to console-next's own HTTP API, as an alternative to a session cookie.
// Only the token's sha256 hash is ever stored - the token itself is shown
// once, at creation, like a GitHub PAT.

import { randomBytes, createHash } from "node:crypto";
import { db } from "../db.js";
import type { User } from "./users.js";

interface ApiKeyRow {
  id: number;
  user_id: number;
  label: string;
  created_at: string;
  last_used_at: string | null;
}

/** ApiKeySummary is what a key's owner may see about it after creation - never the token or its hash. */
export interface ApiKeySummary {
  id: number;
  label: string;
  createdAt: string;
  lastUsedAt: string | null;
}

function hashKey(token: string): string {
  return createHash("sha256").update(token).digest("hex");
}

function toSummary(row: Pick<ApiKeyRow, "id" | "label" | "created_at" | "last_used_at">): ApiKeySummary {
  return { id: row.id, label: row.label, createdAt: row.created_at, lastUsedAt: row.last_used_at };
}

/** createApiKey mints a new bearer token for userId, returning it once - only its hash is persisted. */
export function createApiKey(userId: number, label: string): { id: number; token: string } {
  const token = randomBytes(32).toString("hex");
  const hash = hashKey(token);
  db.prepare("INSERT INTO api_keys (user_id, key_hash, label) VALUES (?, ?, ?)").run(userId, hash, label);
  const row = db.prepare("SELECT id FROM api_keys WHERE key_hash = ?").get(hash) as { id: number };
  return { id: row.id, token };
}

/** listApiKeys returns userId's own keys, newest first - never the token or its hash. */
export function listApiKeys(userId: number): ApiKeySummary[] {
  const rows = db
    .prepare("SELECT id, user_id, label, created_at, last_used_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC")
    .all(userId) as unknown as ApiKeyRow[];
  return rows.map(toSummary);
}

/** revokeApiKey deletes userId's own key by id; a no-op if it belongs to someone else or doesn't exist. */
export function revokeApiKey(userId: number, id: number): void {
  db.prepare("DELETE FROM api_keys WHERE id = ? AND user_id = ?").run(id, userId);
}

/** apiKeyUser resolves a bearer token to its owning user and bumps last_used_at; undefined if the token is unknown. */
export function apiKeyUser(token: string): User | undefined {
  const hash = hashKey(token);
  const row = db
    .prepare("SELECT u.id AS id, u.username AS username, u.is_admin AS is_admin FROM api_keys k JOIN users u ON u.id = k.user_id WHERE k.key_hash = ?")
    .get(hash) as { id: number; username: string; is_admin: number } | undefined;
  if (!row) return undefined;
  db.prepare("UPDATE api_keys SET last_used_at = datetime('now') WHERE key_hash = ?").run(hash);
  return { id: row.id, username: row.username, isAdmin: Number(row.is_admin) === 1 };
}
