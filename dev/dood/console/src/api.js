// All URLs are relative to the console's own mount point (`/` at the root,
// `/gw/abc/` in a child), which is always the document's directory.

async function request(path, opts = {}) {
  const res = await fetch(path, { cache: "no-store", ...opts });
  const type = res.headers.get("content-type") || "";
  const body = type.includes("application/json") ? await res.json() : await res.text();
  if (!res.ok) {
    throw new Error((body && body.error) || `${res.status} ${res.statusText}`);
  }
  return body;
}

const send = (method, path, body) =>
  request(path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

export const api = {
  self: () => request("api/self"),
  gateways: () => request("api/gateways"),
  spawn: (req) =>
    request("api/gateways", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  // config: what happens to the child's config changes - merge | branch
  // (default: saved as a branch) | discard.
  stop: (name, config) =>
    request(`api/gateways/${encodeURIComponent(name)}${config ? `?config=${config}` : ""}`, { method: "DELETE" }),
  branches: () => request("api/branches"),
  deleteBranch: (id) => request(`api/branches/${encodeURIComponent(id)}`, { method: "DELETE" }),
  log: (name) => request(`api/gateways/${encodeURIComponent(name)}/log`),
  // Container logs: our own bundle's services, or a child bundle's.
  serviceLogs: (svc) => request(`api/services/${encodeURIComponent(svc)}/logs`),
  childServiceLogs: (name, svc) =>
    request(`api/gateways/${encodeURIComponent(name)}/services/${encodeURIComponent(svc)}/logs`),
  // Local services: start/stop containers in our own compose project.
  startLocal: (svc, project) =>
    request(`api/services/${encodeURIComponent(svc)}/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: project || "" }),
    }),
  stopLocal: (svc) => request(`api/services/${encodeURIComponent(svc)}/stop`, { method: "POST" }),
  // Workspaces: what gateways work on (workspace.yaml on a manager; the one
  // assigned workspace on a workspace gateway).
  workspaces: () => request("api/workspaces"),
  addWorkspace: (ws) => send("POST", "api/workspaces", ws),
  editWorkspace: (name, ws) => send("PUT", `api/workspaces/${encodeURIComponent(name)}`, ws),
  removeWorkspace: (name, purge) =>
    request(`api/workspaces/${encodeURIComponent(name)}${purge ? "?purge=true" : ""}`, { method: "DELETE" }),
  // A workspace gateway's open work (uncommitted/unpushed), asked of it over NATS.
  workspaceStatus: (name) => request(`api/gateways/${encodeURIComponent(name)}/workspace-status`),
  startWorkspaceGateway: (name, opts) => send("POST", `api/workspaces/${encodeURIComponent(name)}/gateway`, opts || {}),
  // NATS: a gateway's own inbox/send, reached at `base` - that gateway's
  // own GW_PREFIX ("" for this one, or e.g. "/gw/abc" for another one in
  // the cascade, exactly matching api.self()'s "prefix" / a Gateway's
  // "prefix" field). Absolute, so it works for self, a direct child, a
  // parent, or a sibling - anywhere ordinary Traefik routing reaches.
  inbox: (base = "") => request(`${base}/api/nats/inbox`),
  sendMessage: (base, text) => send("POST", `${base}/api/nats/send`, { text }),
  gatewaysAt: (base) => request(`${base}/api/gateways`),
  // Requests up the tree: ours to our parent, and our children's to us
  // (queued until approved here - requests.go).
  askParent: (kind, reason) => send("POST", "api/parent/requests", { kind, reason }),
  approveRequest: (name) => request(`api/requests/${encodeURIComponent(name)}/approve`, { method: "POST" }),
  dismissRequest: (name) => request(`api/requests/${encodeURIComponent(name)}`, { method: "DELETE" }),
  // Jobs: one-shot actions against GitHub, Docker Hub, GCP (all native). `project` is
  // the sidebar's selection ("" / "self" = this repo); `args` is by name.
  jobs: () => request("api/jobs"),
  jobRuns: () => request("api/jobs/runs"),
  runJob: (group, name, project, args) =>
    request(`api/jobs/${encodeURIComponent(group)}/${encodeURIComponent(name)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: project || "", args }),
    }),
  jobLog: (id) => request(`api/jobs/log?id=${encodeURIComponent(id)}`),
  // Deployment: deployment.yaml (Docker Hub images, cloud targets) with each
  // target's last known VM state, and a NATS message to a cloud gateway.
  deployment: () => request("api/deployment"),
  messageTarget: (name, text) => send("POST", `api/deployment/targets/${encodeURIComponent(name)}/message`, { text }),
  // Editing deployment.yaml, section by section (root gateway only).
  editDockerHub: (body) => send("PUT", "api/deployment/docker-hub", body),
  hubRepoStatus: () => request("api/deployment/docker-hub/repo"),
  addImage: (im) => send("POST", "api/deployment/images", im),
  editImage: (name, im) => send("PUT", `api/deployment/images/${encodeURIComponent(name)}`, im),
  removeImage: (name) => request(`api/deployment/images/${encodeURIComponent(name)}`, { method: "DELETE" }),
  editGCP: (body) => send("PUT", "api/deployment/gcp", body),
  addTarget: (t) => send("POST", "api/deployment/targets", t),
  editTarget: (name, t) => send("PUT", `api/deployment/targets/${encodeURIComponent(name)}`, t),
  removeTarget: (name) => request(`api/deployment/targets/${encodeURIComponent(name)}`, { method: "DELETE" }),
};
