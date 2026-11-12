import type { MiddlewareHandler } from "hono";
import { getCookie } from "hono/cookie";
import { sessionUser } from "./sessions.js";
import { apiKeyUser } from "./apiKeys.js";
import type { AppEnv } from "./context.js";

/** SESSION_COOKIE is the name of the opaque session-id cookie set on login. */
export const SESSION_COOKIE = "cn_session";

/**
 * requireAuth is Hono middleware: accepts either an `Authorization: Bearer
 * <key>` service API key (Phase 3) or a session cookie (Phase 2), 401s if
 * neither names a user, else attaches that user to context as "user".
 */
export const requireAuth: MiddlewareHandler<AppEnv> = async (c, next) => {
  const bearer = c.req.header("Authorization")?.match(/^Bearer (.+)$/)?.[1];
  const sessionId = getCookie(c, SESSION_COOKIE);
  const user = (bearer ? apiKeyUser(bearer) : undefined) ?? (sessionId ? sessionUser(sessionId) : undefined);
  if (!user) return c.json({ error: "unauthorized" }, 401);
  c.set("user", user);
  await next();
};
