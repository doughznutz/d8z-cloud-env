// Per-user third-party credentials (a GitHub PAT, for now), encrypted at
// rest with console-next's own master key (src/crypto/secretBox.ts) - a
// separate key and purpose from Go's wsnats.go crypto, never shared with Go.
// Go keeps storing a workspace's token in workspace.yaml exactly as today;
// this table only holds what console-next injects into that existing field
// on the acting user's behalf (see injectGithubToken in src/apiRoutes.ts).

import { db } from "../db.js";
import { encryptSecret, decryptSecret } from "../crypto/secretBox.js";

/** CredentialKind enumerates the third-party secret types console-next stores on a user's behalf. */
export type CredentialKind = "github_pat";

interface CredentialRow {
  id: number;
  user_id: number;
  kind: string;
  label: string;
  created_at: string;
}

/** CredentialSummary is what a credential's owner may see about it - never the decrypted secret. */
export interface CredentialSummary {
  id: number;
  kind: CredentialKind;
  label: string;
  createdAt: string;
}

function toSummary(row: Pick<CredentialRow, "id" | "kind" | "label" | "created_at">): CredentialSummary {
  return { id: row.id, kind: row.kind as CredentialKind, label: row.label, createdAt: row.created_at };
}

/** storeCredential encrypts and saves a new credential for userId, returning its id. */
export function storeCredential(userId: number, kind: CredentialKind, label: string, secret: string): number {
  db.prepare("INSERT INTO credentials (user_id, kind, label, secret_encrypted) VALUES (?, ?, ?, ?)").run(
    userId,
    kind,
    label,
    encryptSecret(secret),
  );
  const row = db.prepare("SELECT id FROM credentials WHERE user_id = ? ORDER BY id DESC LIMIT 1").get(userId) as { id: number };
  return row.id;
}

/** listCredentials returns userId's own credentials, newest first - never the decrypted secret. */
export function listCredentials(userId: number): CredentialSummary[] {
  const rows = db
    .prepare("SELECT id, user_id, kind, label, created_at FROM credentials WHERE user_id = ? ORDER BY created_at DESC")
    .all(userId) as unknown as CredentialRow[];
  return rows.map(toSummary);
}

/** revokeCredential deletes userId's own credential by id; a no-op if it belongs to someone else or doesn't exist. */
export function revokeCredential(userId: number, id: number): void {
  db.prepare("DELETE FROM credentials WHERE id = ? AND user_id = ?").run(id, userId);
}

/** latestCredential returns userId's most recently stored credential of kind, decrypted - undefined if they have none. */
export function latestCredential(userId: number, kind: CredentialKind): string | undefined {
  const row = db
    .prepare("SELECT secret_encrypted FROM credentials WHERE user_id = ? AND kind = ? ORDER BY created_at DESC LIMIT 1")
    .get(userId, kind) as { secret_encrypted: string } | undefined;
  return row ? decryptSecret(row.secret_encrypted) : undefined;
}
