// All URLs are relative to the console's own mount point (`/` at the root,
// `/gw/abc/` in a child), which is always the document's directory.

import type {
  Branch,
  Deployment,
  DockerHubInput,
  Gateway,
  HubRepoStatus,
  ImageInput,
  GCPInput,
  InboxMessage,
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
} from "./types";

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(path, { cache: "no-store", ...opts });
  const type = res.headers.get("content-type") || "";
  const body: unknown = type.includes("application/json") ? await res.json() : await res.text();
  if (!res.ok) {
    const message = body && typeof body === "object" && "error" in body ? String(body.error) : undefined;
    throw new Error(message || `${res.status} ${res.statusText}`);
  }
  return body as T;
}

function send<T>(method: string, path: string, body: unknown): Promise<T> {
  return request<T>(path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
}

export const api = {
  /** This gateway's own identity and live status. */
  self: (): Promise<Self> => request("api/self"),
  /** Every ad hoc child gateway spawned under this one. */
  gateways: (): Promise<{ gateways?: Gateway[]; error?: string }> => request("api/gateways"),
  /** Launches a new ad hoc child gateway. */
  spawn: (req: SpawnRequest): Promise<void> =>
    request("api/gateways", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  /**
   * Stops a child gateway.
   * @param config What to do with its workspace's open work: merge | branch
   * (default: saved as a branch) | discard.
   */
  stop: (name: string, config?: StopAction): Promise<void> =>
    request(`api/gateways/${encodeURIComponent(name)}${config ? `?config=${config}` : ""}`, { method: "DELETE" }),
  /** Branches saved from stopped workspace gateways. */
  branches: (): Promise<{ branches: Branch[] }> => request("api/branches"),
  /** Deletes a saved branch. */
  deleteBranch: (id: string): Promise<void> => request(`api/branches/${encodeURIComponent(id)}`, { method: "DELETE" }),
  /** The spawn/start-stop log for a child gateway. */
  log: (name: string): Promise<string> => request(`api/gateways/${encodeURIComponent(name)}/log`),
  /** Container logs for one of our own bundle's services. */
  serviceLogs: (svc: string): Promise<string> => request(`api/services/${encodeURIComponent(svc)}/logs`),
  /** Container logs for one of a child bundle's services. */
  childServiceLogs: (name: string, svc: string): Promise<string> =>
    request(`api/gateways/${encodeURIComponent(name)}/services/${encodeURIComponent(svc)}/logs`),
  /** Starts a container in our own compose project. */
  startLocal: (svc: string, project?: string): Promise<void> =>
    request(`api/services/${encodeURIComponent(svc)}/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: project || "" }),
    }),
  /** Stops a container in our own compose project. */
  stopLocal: (svc: string): Promise<void> => request(`api/services/${encodeURIComponent(svc)}/stop`, { method: "POST" }),
  /** What gateways work on: workspaces.yaml on a manager, or the one assigned workspace on a workspace gateway. */
  workspaces: (): Promise<WorkspaceInfo> => request("api/workspaces"),
  /** Adds a new workspace. */
  addWorkspace: (ws: WorkspaceInput): Promise<void> => send("POST", "api/workspaces", ws),
  /** Edits an existing workspace's configuration. */
  editWorkspace: (name: string, ws: WorkspaceInput): Promise<void> => send("PUT", `api/workspaces/${encodeURIComponent(name)}`, ws),
  /** Removes a workspace, optionally deleting its local checkout. */
  removeWorkspace: (name: string, purge?: boolean): Promise<void> =>
    request(`api/workspaces/${encodeURIComponent(name)}${purge ? "?purge=true" : ""}`, { method: "DELETE" }),
  /** A workspace gateway's open work (uncommitted/unpushed), asked of it over NATS. */
  workspaceStatus: (name: string): Promise<WorkspaceStatus> => request(`api/gateways/${encodeURIComponent(name)}/workspace-status`),
  /** Starts a gateway for a workspace that doesn't currently have one running. */
  startWorkspaceGateway: (name: string, opts?: StartWorkspaceGatewayOptions): Promise<void> =>
    send("POST", `api/workspaces/${encodeURIComponent(name)}/gateway`, opts || {}),
  /**
   * NATS: a gateway's own inbox, reached at `base` - that gateway's own
   * GW_PREFIX ("" for this one, or e.g. "/gw/abc" for another one in the
   * cascade). Absolute, so it works for self, a direct child, a parent, or
   * a sibling - anywhere ordinary Traefik routing reaches.
   */
  inbox: (base = ""): Promise<{ messages: InboxMessage[] }> => request(`${base}/api/nats/inbox`),
  /** Sends a NATS message from a gateway's own inbox. */
  sendMessage: (base: string, text: string): Promise<void> => send("POST", `${base}/api/nats/send`, { text }),
  /** The gateways visible from `base` (a parent, sibling, or self). */
  gatewaysAt: (base: string): Promise<{ gateways: Gateway[] }> => request(`${base}/api/gateways`),
  /** Requests up the tree: ours to our parent, queued until approved. */
  askParent: (kind: string, reason: string): Promise<void> => send("POST", "api/parent/requests", { kind, reason }),
  /** Approves a child's queued request (parent only). */
  approveRequest: (name: string): Promise<void> => request(`api/requests/${encodeURIComponent(name)}/approve`, { method: "POST" }),
  /** Dismisses a child's queued request without approving it (parent only). */
  dismissRequest: (name: string): Promise<void> => request(`api/requests/${encodeURIComponent(name)}`, { method: "DELETE" }),
  /** The catalog of one-shot jobs available against GitHub, Docker Hub, GCP. */
  jobs: (): Promise<{ jobs?: Job[] }> => request("api/jobs"),
  /** The live/last state of every job run, keyed by `"<group>/<name>"`. */
  jobRuns: (): Promise<{ runs?: Record<string, JobRun> }> => request("api/jobs/runs"),
  /**
   * Runs a job.
   * @param project The sidebar's selection ("" / "self" = this repo).
   * @param args Job arguments, by name.
   */
  runJob: (group: string, name: string, project: string, args: Record<string, string>): Promise<void> =>
    request(`api/jobs/${encodeURIComponent(group)}/${encodeURIComponent(name)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: project || "", args }),
    }),
  /** A job run's captured output. */
  jobLog: (id: string): Promise<string> => request(`api/jobs/log?id=${encodeURIComponent(id)}`),
  /** deployment.yaml (Docker Hub images, cloud targets) with each target's last known VM state. */
  deployment: (): Promise<Deployment> => request("api/deployment"),
  /** Sends a NATS message to a cloud deploy target. */
  messageTarget: (name: string, text: string): Promise<{ reply: string }> =>
    send("POST", `api/deployment/targets/${encodeURIComponent(name)}/message`, { text }),
  /** Edits deployment.yaml's Docker Hub account section (root gateway only). */
  editDockerHub: (body: DockerHubInput): Promise<void> => send("PUT", "api/deployment/docker-hub", body),
  /** Checks whether the configured Docker Hub repository exists, and its visibility. */
  hubRepoStatus: (): Promise<HubRepoStatus> => request("api/deployment/docker-hub/repo"),
  /** Adds a Docker Hub image entry. */
  addImage: (im: ImageInput): Promise<void> => send("POST", "api/deployment/images", im),
  /** Edits a Docker Hub image entry. */
  editImage: (name: string, im: ImageInput): Promise<void> => send("PUT", `api/deployment/images/${encodeURIComponent(name)}`, im),
  /** Removes a Docker Hub image entry. */
  removeImage: (name: string): Promise<void> => request(`api/deployment/images/${encodeURIComponent(name)}`, { method: "DELETE" }),
  /** Edits deployment.yaml's GCP project/credentials section (root gateway only). */
  editGCP: (body: GCPInput): Promise<void> => send("PUT", "api/deployment/gcp", body),
  /** Adds a cloud deploy target. */
  addTarget: (t: TargetInput): Promise<void> => send("POST", "api/deployment/targets", t),
  /** Edits a cloud deploy target. */
  editTarget: (name: string, t: TargetInput): Promise<void> => send("PUT", `api/deployment/targets/${encodeURIComponent(name)}`, t),
  /** Removes a cloud deploy target. */
  removeTarget: (name: string): Promise<void> => request(`api/deployment/targets/${encodeURIComponent(name)}`, { method: "DELETE" }),
};
