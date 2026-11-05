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
  stop: (name) => request(`api/gateways/${encodeURIComponent(name)}`, { method: "DELETE" }),
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
  // Jobs: one-shot actions against GitHub, Docker Hub, GCP. `project` is
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
};
