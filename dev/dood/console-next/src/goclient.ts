/**
 * Typed, server-side client for gw-server's HTTP API - console-next's own
 * sibling to console/src/api.ts, used internally by this server's routes
 * (and, from Phase 2 on, by auth/ACL logic that needs to inspect or
 * transform what Go returns before relaying it to the browser).
 *
 * Three of console/src/api.ts's methods aren't mirrored here - inbox,
 * sendMessage, gatewaysAt - because they're addressed by an arbitrary `base`
 * prefix into another gateway in the cascade, not a fixed path; nothing
 * transforms them yet, so they pass straight through src/proxy.ts untouched.
 */

import { loadConfig } from "./config.js";
import type {
  Branch,
  Deployment,
  DockerHubInput,
  Gateway,
  GCPInput,
  HubRepoStatus,
  ImageInput,
  Job,
  JobRun,
  Self,
  SpawnRequest,
  StartWorkspaceGatewayOptions,
  StopAction,
  TargetInput,
  WorkspaceInfo,
  WorkspaceInput,
  WorkspaceStatus,
} from "./types.js";

const cfg = loadConfig();

/**
 * GoResponse is what the Go server answered: status, content-type, and the
 * parsed body (JSON if the content-type says so, text otherwise). Unlike
 * console/src/api.ts's request() - which collapses a non-2xx into a thrown
 * Error - this keeps the status so callers can relay it to the browser
 * faithfully instead of silently turning every failure into a 200.
 */
export interface GoResponse<T> {
  status: number;
  contentType: string;
  body: T;
}

/** goRequest issues one HTTP request to the Go gw-server this console-next instance fronts. */
async function goRequest<T>(path: string, init: RequestInit = {}): Promise<GoResponse<T>> {
  const headers = new Headers(init.headers);
  if (cfg.authHeader && !headers.has("Authorization")) headers.set("Authorization", cfg.authHeader);
  // No cache: no-store equivalent needed - undici's fetch has no implicit
  // HTTP cache to opt out of, unlike a browser's.
  const res = await fetch(new URL(path, cfg.upstream), { ...init, headers });
  const contentType = res.headers.get("content-type") || "";
  const body = (contentType.includes("application/json") ? await res.json() : await res.text()) as T;
  return { status: res.status, contentType, body };
}

/** goSend is goRequest for a JSON request body, mirroring console/src/api.ts's send(). */
function goSend<T>(method: string, path: string, body: unknown): Promise<GoResponse<T>> {
  return goRequest<T>(path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
}

/** goclient mirrors console/src/api.ts's surface, one function per Go route, for console-next's own server-side use. */
export const goclient = {
  /** @route GET /api/self */
  self: (): Promise<GoResponse<Self>> => goRequest("api/self"),
  /** @route GET /api/gateways */
  gateways: (): Promise<GoResponse<{ gateways?: Gateway[]; error?: string }>> => goRequest("api/gateways"),
  /** @route POST /api/gateways */
  spawn: (req: SpawnRequest): Promise<GoResponse<unknown>> => goSend("POST", "api/gateways", req),
  /** @route DELETE /api/gateways/{name} */
  stop: (name: string, config?: StopAction): Promise<GoResponse<unknown>> =>
    goRequest(`api/gateways/${encodeURIComponent(name)}${config ? `?config=${config}` : ""}`, { method: "DELETE" }),
  /** @route GET /api/branches */
  branches: (): Promise<GoResponse<{ branches: Branch[] }>> => goRequest("api/branches"),
  /** @route DELETE /api/branches/{id} */
  deleteBranch: (id: string): Promise<GoResponse<unknown>> => goRequest(`api/branches/${encodeURIComponent(id)}`, { method: "DELETE" }),
  /** @route GET /api/gateways/{name}/log */
  log: (name: string): Promise<GoResponse<string>> => goRequest(`api/gateways/${encodeURIComponent(name)}/log`),
  /** @route GET /api/services/{svc}/logs */
  serviceLogs: (svc: string): Promise<GoResponse<string>> => goRequest(`api/services/${encodeURIComponent(svc)}/logs`),
  /** @route GET /api/gateways/{name}/services/{svc}/logs */
  childServiceLogs: (name: string, svc: string): Promise<GoResponse<string>> =>
    goRequest(`api/gateways/${encodeURIComponent(name)}/services/${encodeURIComponent(svc)}/logs`),
  /** @route POST /api/services/{svc}/start */
  startLocal: (svc: string, project?: string): Promise<GoResponse<unknown>> =>
    goSend("POST", `api/services/${encodeURIComponent(svc)}/start`, { project: project || "" }),
  /** @route POST /api/services/{svc}/stop */
  stopLocal: (svc: string): Promise<GoResponse<unknown>> => goRequest(`api/services/${encodeURIComponent(svc)}/stop`, { method: "POST" }),
  /** @route GET /api/workspaces */
  workspaces: (): Promise<GoResponse<WorkspaceInfo>> => goRequest("api/workspaces"),
  /** @route POST /api/workspaces */
  addWorkspace: (ws: WorkspaceInput): Promise<GoResponse<unknown>> => goSend("POST", "api/workspaces", ws),
  /** @route PUT /api/workspaces/{name} */
  editWorkspace: (name: string, ws: WorkspaceInput): Promise<GoResponse<unknown>> =>
    goSend("PUT", `api/workspaces/${encodeURIComponent(name)}`, ws),
  /** @route DELETE /api/workspaces/{name} */
  removeWorkspace: (name: string, purge?: boolean): Promise<GoResponse<unknown>> =>
    goRequest(`api/workspaces/${encodeURIComponent(name)}${purge ? "?purge=true" : ""}`, { method: "DELETE" }),
  /** @route GET /api/gateways/{name}/workspace-status */
  workspaceStatus: (name: string): Promise<GoResponse<WorkspaceStatus>> =>
    goRequest(`api/gateways/${encodeURIComponent(name)}/workspace-status`),
  /** @route POST /api/workspaces/{name}/gateway */
  startWorkspaceGateway: (name: string, opts?: StartWorkspaceGatewayOptions): Promise<GoResponse<unknown>> =>
    goSend("POST", `api/workspaces/${encodeURIComponent(name)}/gateway`, opts || {}),
  /** @route POST /api/parent/requests */
  askParent: (kind: string, reason: string): Promise<GoResponse<unknown>> => goSend("POST", "api/parent/requests", { kind, reason }),
  /** @route POST /api/requests/{name}/approve */
  approveRequest: (name: string): Promise<GoResponse<unknown>> =>
    goRequest(`api/requests/${encodeURIComponent(name)}/approve`, { method: "POST" }),
  /** @route DELETE /api/requests/{name} */
  dismissRequest: (name: string): Promise<GoResponse<unknown>> =>
    goRequest(`api/requests/${encodeURIComponent(name)}`, { method: "DELETE" }),
  /** @route GET /api/jobs */
  jobs: (): Promise<GoResponse<{ jobs?: Job[] }>> => goRequest("api/jobs"),
  /** @route GET /api/jobs/runs */
  jobRuns: (): Promise<GoResponse<{ runs?: Record<string, JobRun> }>> => goRequest("api/jobs/runs"),
  /** @route POST /api/jobs/{group}/{name} */
  runJob: (group: string, name: string, project: string, args: Record<string, string>): Promise<GoResponse<unknown>> =>
    goSend("POST", `api/jobs/${encodeURIComponent(group)}/${encodeURIComponent(name)}`, { project: project || "", args }),
  /** @route GET /api/jobs/log */
  jobLog: (id: string): Promise<GoResponse<string>> => goRequest(`api/jobs/log?id=${encodeURIComponent(id)}`),
  /** @route GET /api/deployment */
  deployment: (): Promise<GoResponse<Deployment>> => goRequest("api/deployment"),
  /** @route POST /api/deployment/targets/{name}/message */
  messageTarget: (name: string, text: string): Promise<GoResponse<{ reply: string }>> =>
    goSend("POST", `api/deployment/targets/${encodeURIComponent(name)}/message`, { text }),
  /** @route PUT /api/deployment/docker-hub */
  editDockerHub: (body: DockerHubInput): Promise<GoResponse<unknown>> => goSend("PUT", "api/deployment/docker-hub", body),
  /** @route GET /api/deployment/docker-hub/repo */
  hubRepoStatus: (): Promise<GoResponse<HubRepoStatus>> => goRequest("api/deployment/docker-hub/repo"),
  /** @route POST /api/deployment/images */
  addImage: (im: ImageInput): Promise<GoResponse<unknown>> => goSend("POST", "api/deployment/images", im),
  /** @route PUT /api/deployment/images/{name} */
  editImage: (name: string, im: ImageInput): Promise<GoResponse<unknown>> =>
    goSend("PUT", `api/deployment/images/${encodeURIComponent(name)}`, im),
  /** @route DELETE /api/deployment/images/{name} */
  removeImage: (name: string): Promise<GoResponse<unknown>> =>
    goRequest(`api/deployment/images/${encodeURIComponent(name)}`, { method: "DELETE" }),
  /** @route PUT /api/deployment/gcp */
  editGCP: (body: GCPInput): Promise<GoResponse<unknown>> => goSend("PUT", "api/deployment/gcp", body),
  /** @route POST /api/deployment/targets */
  addTarget: (t: TargetInput): Promise<GoResponse<unknown>> => goSend("POST", "api/deployment/targets", t),
  /** @route PUT /api/deployment/targets/{name} */
  editTarget: (name: string, t: TargetInput): Promise<GoResponse<unknown>> =>
    goSend("PUT", `api/deployment/targets/${encodeURIComponent(name)}`, t),
  /** @route DELETE /api/deployment/targets/{name} */
  removeTarget: (name: string): Promise<GoResponse<unknown>> =>
    goRequest(`api/deployment/targets/${encodeURIComponent(name)}`, { method: "DELETE" }),
};
