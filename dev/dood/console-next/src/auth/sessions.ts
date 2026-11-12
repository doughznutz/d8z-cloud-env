// Server-side sessions: an opaque random cookie value keyed to a row in
// SQLite, chosen over a JWT so logout/revocation is an instant DELETE
// instead of a denylist - see the plan's rationale (single-box front door,
// instant revocation matters more than statelessness here).

import { randomBytes } from "node:crypto";
import { db } from "../db.js";
import type { User } from "./users.js";

const SESSION_TTL_MS = 7 * 24 * 60 * 60 * 1000; // 7 days

/** createSession makes a new opaque session for user and returns its id (the cookie value). */
export function createSession(user: User): string {
  const id = randomBytes(32).toString("hex");
  const expiresAt = new Date(Date.now() + SESSION_TTL_MS).toISOString();
  db.prepare("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)").run(id, user.id, expiresAt);
  return id;
}

/** sessionUser resolves a session id to its User, if the session exists and hasn't expired. */
export function sessionUser(sessionId: string): User | undefined {
  const row = db
    .prepare(
      `SELECT u.id AS id, u.username AS username, u.is_admin AS is_admin FROM sessions s
       JOIN users u ON u.id = s.user_id
       WHERE s.id = ? AND s.expires_at > datetime('now')`,
    )
    .get(sessionId) as { id: number; username: string; is_admin: number } | undefined;
  if (!row) return undefined;
  return { id: row.id, username: row.username, isAdmin: Number(row.is_admin) === 1 };
}

/** deleteSession removes a session (logout). */
export function deleteSession(sessionId: string): void {
  db.prepare("DELETE FROM sessions WHERE id = ?").run(sessionId);
}
