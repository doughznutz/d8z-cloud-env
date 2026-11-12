import { serve } from "@hono/node-server";
import { Hono } from "hono";
import { loadConfig } from "./config.js";
import { registerApiRoutes } from "./apiRoutes.js";
import { registerAdminRoutes } from "./adminRoutes.js";
import { registerSelfServiceRoutes } from "./selfServiceRoutes.js";
import { registerPublicAuthRoutes, registerProtectedAuthRoutes } from "./authRoutes.js";
import { requireAuth } from "./auth/middleware.js";
import { bootstrapAdmin } from "./auth/bootstrap.js";
import type { AppEnv } from "./auth/context.js";
import { proxyPass } from "./proxy.js";

bootstrapAdmin();

const cfg = loadConfig();
const app = new Hono<AppEnv>();

app.get("/healthz", (c) => c.text("ok"));
// /auth/login must be reachable without a session; everything registered
// after requireAuth below may assume c.get("user") is set.
registerPublicAuthRoutes(app);
// So must the console's own shell and bundle - its login form lives there
// (console/src/Auth.tsx). Static files only; every API call stays gated.
app.get("/", (c) => proxyPass(c));
app.get("/index.html", (c) => proxyPass(c));
app.get("/assets/*", (c) => proxyPass(c));
app.use("*", requireAuth);
registerProtectedAuthRoutes(app);
registerAdminRoutes(app);
registerSelfServiceRoutes(app);
registerApiRoutes(app);
// Everything not matched above - the base-relative NATS/gatewaysAt calls,
// /deightz, /og, /dozzle, and nested /gw/<name>/... mounts - is Traefik's
// job to route, not gw-server's, so it passes straight through unmodified
// (still behind requireAuth, same as every other path here).
app.all("*", (c) => proxyPass(c));

const [hostname, portStr] = cfg.listen.split(":");
const port = Number(portStr);

serve({ fetch: app.fetch, hostname, port }, (info) => {
  console.log(`console-next listening on ${info.address}:${info.port}, proxying to ${cfg.gwTarget}`);
});
