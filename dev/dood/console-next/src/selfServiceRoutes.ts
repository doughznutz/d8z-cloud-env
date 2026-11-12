/**
 * Self-service endpoints for a signed-in user's own service API keys and
 * stored third-party credentials (Phase 3). Registered after requireAuth;
 * every handler scopes strictly to c.get("user").id - there is no admin
 * override here, since these are a user's own secrets, not something an
 * admin manages on their behalf.
 */

import type { Hono } from "hono";
import { createApiKey, listApiKeys, revokeApiKey } from "./auth/apiKeys.js";
import { storeCredential, listCredentials, revokeCredential, type CredentialKind } from "./auth/credentials.js";
import type { AppEnv } from "./auth/context.js";

const CREDENTIAL_KINDS: CredentialKind[] = ["github_pat"];

interface CreateKeyBody {
  label?: string;
}

interface CreateCredentialBody {
  kind?: string;
  label?: string;
  secret?: string;
}

/** registerSelfServiceRoutes wires /keys and /credentials, each scoped to the signed-in user. */
export function registerSelfServiceRoutes(app: Hono<AppEnv>): void {
  app.get("/keys", (c) => c.json(listApiKeys(c.get("user").id)));
  app.post("/keys", async (c) => {
    const body = await c.req.json<CreateKeyBody>().catch((): CreateKeyBody => ({}));
    const { id, token } = createApiKey(c.get("user").id, body.label || "unlabeled");
    return c.json({ id, token }, 201);
  });
  app.delete("/keys/:id", (c) => {
    revokeApiKey(c.get("user").id, Number(c.req.param("id")));
    return c.json({ ok: true });
  });

  app.get("/credentials", (c) => c.json(listCredentials(c.get("user").id)));
  app.post("/credentials", async (c) => {
    const body = await c.req.json<CreateCredentialBody>().catch((): CreateCredentialBody => ({}));
    if (!body.kind || !CREDENTIAL_KINDS.includes(body.kind as CredentialKind) || !body.secret) {
      return c.json({ error: "kind (github_pat) and secret required" }, 400);
    }
    const id = storeCredential(c.get("user").id, body.kind as CredentialKind, body.label || "default", body.secret);
    return c.json({ id }, 201);
  });
  app.delete("/credentials/:id", (c) => {
    revokeCredential(c.get("user").id, Number(c.req.param("id")));
    return c.json({ ok: true });
  });
}
