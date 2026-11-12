// Per-user workspace scoping: who may see which workspace. Lives entirely
// in console-next's own SQLite (not a field on Go's Workspace struct) per
// the plan - loosely joined against Go's workspace.yaml by name, revisited
// only if that drift proves painful in practice.

import { db } from "../db.js";

/** Role is how a user relates to a workspace they've been granted access to. */
export type Role = "owner" | "collaborator" | "viewer";

/** grantWorkspaceAccess upserts a user's role on a workspace (by name). */
export function grantWorkspaceAccess(userId: number, workspaceName: string, role: Role): void {
  db.prepare(
    `INSERT INTO workspace_acl (user_id, workspace_name, role) VALUES (?, ?, ?)
     ON CONFLICT(user_id, workspace_name) DO UPDATE SET role = excluded.role`,
  ).run(userId, workspaceName, role);
}

/** revokeWorkspaceAccess removes a user's access row for a workspace. */
export function revokeWorkspaceAccess(userId: number, workspaceName: string): void {
  db.prepare("DELETE FROM workspace_acl WHERE user_id = ? AND workspace_name = ?").run(userId, workspaceName);
}

/** allowedWorkspaceNames returns every workspace name userId has any ACL row for. */
export function allowedWorkspaceNames(userId: number): Set<string> {
  const rows = db.prepare("SELECT workspace_name FROM workspace_acl WHERE user_id = ?").all(userId) as { workspace_name: string }[];
  return new Set(rows.map((r) => r.workspace_name));
}
