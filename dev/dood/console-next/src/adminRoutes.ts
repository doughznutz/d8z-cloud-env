/**
 * Admin-only management endpoints for the two things Phase 2 introduces
 * that have no other way to populate yet: user accounts, and per-user
 * workspace ACL rows. Registered after requireAuth, so c.get("user") names
 * the acting user, checked against isAdmin on every handler.
 */

import type { Hono } from "hono";
import { createUser, getUserByUsername } from "./auth/users.js";
import { grantWorkspaceAccess, revokeWorkspaceAccess, type Role } from "./auth/acl.js";
import { setDockerPolicy, type DockerMode } from "./auth/dockerPolicy.js";
import type { AppEnv } from "./auth/context.js";

interface CreateUserBody {
  username?: string;
  password?: string;
  isAdmin?: boolean;
}

interface GrantBody {
  role?: Role;
}

interface DockerPolicyBody {
  mode?: DockerMode;
}

/** registerAdminRoutes wires /admin/users and /admin/acl/{workspace}/{username}, every handler gated on c.get("user").isAdmin. */
export function registerAdminRoutes(app: Hono<AppEnv>): void {
  app.post("/admin/users", async (c) => {
    if (!c.get("user").isAdmin) return c.json({ error: "forbidden" }, 403);
    const body = await c.req.json<CreateUserBody>().catch((): CreateUserBody => ({}));
    if (!body.username || !body.password) return c.json({ error: "username and password required" }, 400);
    try {
      const user = createUser(body.username, body.password, body.isAdmin ?? false);
      return c.json({ username: user.username, isAdmin: user.isAdmin }, 201);
    } catch {
      return c.json({ error: "username already taken" }, 409);
    }
  });

  app.put("/admin/acl/:workspace/:username", async (c) => {
    if (!c.get("user").isAdmin) return c.json({ error: "forbidden" }, 403);
    const target = getUserByUsername(c.req.param("username"));
    if (!target) return c.json({ error: "no such user" }, 404);
    const body = await c.req.json<GrantBody>().catch((): GrantBody => ({}));
    grantWorkspaceAccess(target.id, c.req.param("workspace"), body.role || "viewer");
    return c.json({ ok: true });
  });

  app.delete("/admin/acl/:workspace/:username", (c) => {
    if (!c.get("user").isAdmin) return c.json({ error: "forbidden" }, 403);
    const target = getUserByUsername(c.req.param("username"));
    if (!target) return c.json({ error: "no such user" }, 404);
    revokeWorkspaceAccess(target.id, c.req.param("workspace"));
    return c.json({ ok: true });
  });

  app.put("/admin/users/:username/docker-policy", async (c) => {
    if (!c.get("user").isAdmin) return c.json({ error: "forbidden" }, 403);
    const target = getUserByUsername(c.req.param("username"));
    if (!target) return c.json({ error: "no such user" }, 404);
    const body = await c.req.json<DockerPolicyBody>().catch((): DockerPolicyBody => ({}));
    if (body.mode !== "host" && body.mode !== "dind") return c.json({ error: "mode must be host or dind" }, 400);
    setDockerPolicy(target.id, body.mode);
    return c.json({ ok: true });
  });
}
