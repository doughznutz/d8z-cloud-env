/**
 * Registers one Hono route per fixed-path Go route (console/src/api.ts's
 * surface, minus the three base-relative NATS/gatewaysAt calls - see
 * src/proxy.ts), each just calling goclient and relaying the result with
 * the same status/content-type Go sent, no transformation yet. Phase 2+
 * logic (auth, ACL filtering, credential injection) replaces individual
 * handlers here without touching the rest.
 */

import type { Hono } from "hono";
import { goclient, type GoResponse } from "./goclient.js";
import { allowedWorkspaceNames } from "./auth/acl.js";
import { latestCredential } from "./auth/credentials.js";
import { getDockerPolicy } from "./auth/dockerPolicy.js";
import { natsInbox, natsSend } from "./natsClient.js";
import type { AppEnv } from "./auth/context.js";
import type {
  SpawnRequest,
  StopAction,
  WorkspaceInput,
  StartWorkspaceGatewayOptions,
  DockerHubInput,
  ImageInput,
  GCPInput,
  TargetInput,
} from "./types.js";

/**
 * applyDockerPolicy overwrites a request's docker field with the acting
 * user's own console-next-owned policy (Phase 5) - whatever a browser form
 * sent is advisory at best, since this decision belongs to console-next,
 * not to whoever filled in the form.
 */
function applyDockerPolicy<T extends { docker: "host" | "dind" }>(req: T, userId: number): T {
  return { ...req, docker: getDockerPolicy(userId) };
}

/**
 * injectGithubToken fills ws.token from the acting user's stored GitHub PAT
 * (Phase 3's credentials store) when the caller is adding/editing a
 * github-sourced workspace and didn't supply a token of their own - Go's
 * existing field, just populated from a different place.
 */
function injectGithubToken(ws: WorkspaceInput, userId: number): WorkspaceInput {
  if (ws.source !== "github" || ws.token || ws.clearToken) return ws;
  const token = latestCredential(userId, "github_pat");
  return token ? { ...ws, token } : ws;
}

let selfIdCache: string | undefined;

/** selfId is this gateway's own GW_ID (Self.id, via goclient.self()), cached since it never changes at runtime - needed to address src/natsClient.ts's subjects/streams, which are keyed by id rather than by this server's own notion of "self". */
async function selfId(): Promise<string | undefined> {
  if (selfIdCache) return selfIdCache;
  const r = await goclient.self();
  if (r.status === 200 && r.contentType.includes("application/json")) selfIdCache = r.body.id;
  return selfIdCache;
}

/** respond turns a GoResponse into the exact HTTP response Go sent: same status, same content-type. */
function respond<T>(r: GoResponse<T>): Response {
  const isJSON = r.contentType.includes("application/json");
  const payload = isJSON ? JSON.stringify(r.body) : String(r.body);
  return new Response(payload, {
    status: r.status,
    headers: { "content-type": r.contentType || (isJSON ? "application/json" : "text/plain; charset=utf-8") },
  });
}

/** registerApiRoutes wires every fixed-path Go route onto app, ahead of the catch-all proxy in src/server.ts. Registered after requireAuth, so every handler may read c.get("user"). */
export function registerApiRoutes(app: Hono<AppEnv>): void {
  app.get("/api/self", async () => respond(await goclient.self()));
  // NATS: this gateway's own inbox, read/sent directly over src/natsClient.ts
  // (console-next's own NATS connection to Go's plain client port) instead
  // of relaying through Go's HTTP /api/nats/* bridge - same subject/stream
  // Go itself uses, just a different transport. Base-relative calls to a
  // *different* gateway's inbox (Messenger's parent/sibling view) still
  // fall through to the generic proxy below, unaffected by this.
  app.get("/api/nats/inbox", async (c) => {
    const id = await selfId();
    if (!id) return c.json({ error: "nats not available" }, 503);
    const maxParam = c.req.query("max");
    try {
      return c.json({ messages: await natsInbox(id, maxParam ? Number(maxParam) : 50) });
    } catch {
      return c.json({ error: "nats not available" }, 503);
    }
  });
  app.post("/api/nats/send", async (c) => {
    const id = await selfId();
    if (!id) return c.json({ error: "nats not available" }, 503);
    const body = await c.req.json<{ text?: string }>().catch((): { text?: string } => ({}));
    if (!body.text) return c.json({ error: "text required" }, 400);
    try {
      await natsSend(id, body.text);
      return c.json({ state: "sent" }, 202);
    } catch {
      return c.json({ error: "nats not available" }, 503);
    }
  });
  app.get("/api/gateways", async () => respond(await goclient.gateways()));
  app.post("/api/gateways", async (c) =>
    respond(await goclient.spawn(applyDockerPolicy(await c.req.json<SpawnRequest>(), c.get("user").id))),
  );
  app.delete("/api/gateways/:name", async (c) => respond(await goclient.stop(c.req.param("name"), c.req.query("config") as StopAction | undefined)));
  app.get("/api/branches", async () => respond(await goclient.branches()));
  app.delete("/api/branches/:id", async (c) => respond(await goclient.deleteBranch(c.req.param("id"))));
  app.get("/api/gateways/:name/log", async (c) => respond(await goclient.log(c.req.param("name"))));
  app.get("/api/services/:svc/logs", async (c) => respond(await goclient.serviceLogs(c.req.param("svc"))));
  app.get("/api/gateways/:name/services/:svc/logs", async (c) =>
    respond(await goclient.childServiceLogs(c.req.param("name"), c.req.param("svc"))),
  );
  app.post("/api/services/:svc/start", async (c) => {
    const body = await c.req.json<{ project?: string }>().catch((): { project?: string } => ({}));
    return respond(await goclient.startLocal(c.req.param("svc"), body.project));
  });
  app.post("/api/services/:svc/stop", async (c) => respond(await goclient.stopLocal(c.req.param("svc"))));
  app.get("/api/workspaces", async (c) => {
    const r = await goclient.workspaces();
    if (r.status !== 200 || !r.contentType.includes("application/json") || c.get("user").isAdmin) return respond(r);
    const allowed = allowedWorkspaceNames(c.get("user").id);
    return respond({ ...r, body: { ...r.body, workspaces: r.body.workspaces.filter((w) => allowed.has(w.name)) } });
  });
  app.post("/api/workspaces", async (c) => {
    const userId = c.get("user").id;
    const ws = applyDockerPolicy(injectGithubToken(await c.req.json<WorkspaceInput>(), userId), userId);
    return respond(await goclient.addWorkspace(ws));
  });
  app.put("/api/workspaces/:name", async (c) => {
    const userId = c.get("user").id;
    const ws = applyDockerPolicy(injectGithubToken(await c.req.json<WorkspaceInput>(), userId), userId);
    return respond(await goclient.editWorkspace(c.req.param("name"), ws));
  });
  app.delete("/api/workspaces/:name", async (c) => respond(await goclient.removeWorkspace(c.req.param("name"), c.req.query("purge") === "true")));
  app.get("/api/gateways/:name/workspace-status", async (c) => respond(await goclient.workspaceStatus(c.req.param("name"))));
  app.post("/api/workspaces/:name/gateway", async (c) => {
    const opts = await c.req.json<StartWorkspaceGatewayOptions>().catch((): StartWorkspaceGatewayOptions => ({}));
    return respond(await goclient.startWorkspaceGateway(c.req.param("name"), opts));
  });
  app.post("/api/parent/requests", async (c) => {
    const body = await c.req.json<{ kind: string; reason: string }>();
    return respond(await goclient.askParent(body.kind, body.reason));
  });
  app.post("/api/requests/:name/approve", async (c) => respond(await goclient.approveRequest(c.req.param("name"))));
  app.delete("/api/requests/:name", async (c) => respond(await goclient.dismissRequest(c.req.param("name"))));
  app.get("/api/jobs", async () => respond(await goclient.jobs()));
  app.get("/api/jobs/runs", async () => respond(await goclient.jobRuns()));
  app.post("/api/jobs/:group/:name", async (c) => {
    const body = await c.req.json<{ project?: string; args: Record<string, string> }>();
    return respond(await goclient.runJob(c.req.param("group"), c.req.param("name"), body.project || "", body.args));
  });
  app.get("/api/jobs/log", async (c) => respond(await goclient.jobLog(c.req.query("id") || "")));
  app.get("/api/deployment", async () => respond(await goclient.deployment()));
  app.post("/api/deployment/targets/:name/message", async (c) => {
    const body = await c.req.json<{ text: string }>();
    return respond(await goclient.messageTarget(c.req.param("name"), body.text));
  });
  app.put("/api/deployment/docker-hub", async (c) => respond(await goclient.editDockerHub(await c.req.json<DockerHubInput>())));
  app.get("/api/deployment/docker-hub/repo", async () => respond(await goclient.hubRepoStatus()));
  app.post("/api/deployment/images", async (c) => respond(await goclient.addImage(await c.req.json<ImageInput>())));
  app.put("/api/deployment/images/:name", async (c) => respond(await goclient.editImage(c.req.param("name"), await c.req.json<ImageInput>())));
  app.delete("/api/deployment/images/:name", async (c) => respond(await goclient.removeImage(c.req.param("name"))));
  app.put("/api/deployment/gcp", async (c) => respond(await goclient.editGCP(await c.req.json<GCPInput>())));
  app.post("/api/deployment/targets", async (c) => respond(await goclient.addTarget(await c.req.json<TargetInput>())));
  app.put("/api/deployment/targets/:name", async (c) => respond(await goclient.editTarget(c.req.param("name"), await c.req.json<TargetInput>())));
  app.delete("/api/deployment/targets/:name", async (c) => respond(await goclient.removeTarget(c.req.param("name"))));
}
