/**
 * /auth/* - login (public, registered before the auth middleware) and
 * logout/me (require an existing session, registered after it). See
 * src/server.ts for the registration order that makes that split work.
 */

import type { Hono } from "hono";
import { getCookie, setCookie, deleteCookie } from "hono/cookie";
import { verifyLogin } from "./auth/users.js";
import { createSession, deleteSession } from "./auth/sessions.js";
import { getDockerPolicy } from "./auth/dockerPolicy.js";
import { SESSION_COOKIE } from "./auth/middleware.js";
import type { AppEnv } from "./auth/context.js";

interface LoginBody {
  username?: string;
  password?: string;
}

const SESSION_MAX_AGE_SECONDS = 7 * 24 * 60 * 60;

/** registerPublicAuthRoutes wires /auth/login - must be reachable without a session, so it's registered ahead of requireAuth. */
export function registerPublicAuthRoutes(app: Hono<AppEnv>): void {
  app.post("/auth/login", async (c) => {
    const body = await c.req.json<LoginBody>().catch((): LoginBody => ({}));
    if (!body.username || !body.password) return c.json({ error: "username and password required" }, 400);
    const user = verifyLogin(body.username, body.password);
    if (!user) return c.json({ error: "invalid credentials" }, 401);
    const sessionId = createSession(user);
    setCookie(c, SESSION_COOKIE, sessionId, {
      httpOnly: true,
      sameSite: "Lax",
      // Follow the request's actual scheme (Traefik sets X-Forwarded-Proto):
      // a Secure cookie on plain http is dropped by the browser everywhere
      // but localhost, so login "succeeds" and every call after it 401s.
      secure: (c.req.header("x-forwarded-proto") ?? new URL(c.req.url).protocol.replace(":", "")) === "https",
      path: "/",
      maxAge: SESSION_MAX_AGE_SECONDS,
    });
    return c.json({ username: user.username, isAdmin: user.isAdmin, dockerPolicy: getDockerPolicy(user.id) });
  });
}

/** registerProtectedAuthRoutes wires /auth/logout and /auth/me - registered after requireAuth, so c.get("user") is always set. */
export function registerProtectedAuthRoutes(app: Hono<AppEnv>): void {
  app.post("/auth/logout", (c) => {
    const sessionId = getCookie(c, SESSION_COOKIE);
    if (sessionId) deleteSession(sessionId);
    deleteCookie(c, SESSION_COOKIE, { path: "/" });
    return c.json({ ok: true });
  });
  app.get("/auth/me", (c) => {
    const user = c.get("user");
    return c.json({ username: user.username, isAdmin: user.isAdmin, dockerPolicy: getDockerPolicy(user.id) });
  });
}
