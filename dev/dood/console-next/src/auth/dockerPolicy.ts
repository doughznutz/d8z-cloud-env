// Per-user docker-mode policy: console-next, not a browser form, decides
// whether a user's spawned gateways/workspaces run "host" (shared daemon)
// or "dind" (isolated sidecar daemon) - see server/gateways.go:55 and
// workspaces.go:56, where Docker is already a caller-supplied field Go
// just relays. Every caller into those fields (src/apiRoutes.ts's
// spawn/addWorkspace/editWorkspace) overwrites whatever a browser sent
// with this policy before forwarding to Go, so the decision is owned here
// regardless of what any UI form offers.

import { db } from "../db.js";

/** DockerMode mirrors Go's own "host"|"dind" Docker field. */
export type DockerMode = "host" | "dind";

/** getDockerPolicy returns userId's docker-mode policy - "host" (the safe default: a shared daemon, no sidecar) unless an admin has set "dind". */
export function getDockerPolicy(userId: number): DockerMode {
  const row = db.prepare("SELECT docker_policy FROM users WHERE id = ?").get(userId) as { docker_policy: string } | undefined;
  return row?.docker_policy === "dind" ? "dind" : "host";
}

/** setDockerPolicy sets userId's docker-mode policy - admin-only, see adminRoutes.ts. */
export function setDockerPolicy(userId: number, mode: DockerMode): void {
  db.prepare("UPDATE users SET docker_policy = ? WHERE id = ?").run(mode, userId);
}
