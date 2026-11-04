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
  // Projects: sibling source trees under ~/projects.
  projects: () => request("api/projects"),
  createProject: (req) =>
    request("api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  startProject: (name) => request(`api/projects/${encodeURIComponent(name)}/up`, { method: "POST" }),
  stopProject: (name) => request(`api/projects/${encodeURIComponent(name)}`, { method: "DELETE" }),
  projectLog: (name) => request(`api/projects/${encodeURIComponent(name)}/log`),
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
