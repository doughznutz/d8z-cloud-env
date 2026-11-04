import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api.js";

// Sub-apps this console knows how to show; which ones appear comes from
// the gateway's own service list (api/self). `compose` is the bundle
// service backing it, for health and logs.
const SERVICES = {
  deightz: { ico: "⌨", name: "Deightz", desc: "Code-server IDE", compose: "deightz" },
  og: { ico: "✱", name: "OG", desc: "Ollama gateway", compose: "ollama-gatewai" },
  gopher: { ico: "✓", name: "Gopher", desc: "Cluster test runner", compose: "gopher" },
  dozzle: { ico: "≡", name: "Dozzle", desc: "Container logs", compose: "dozzle" },
};

const POLL_MS = 3000;
const SELF_POLL_MS = 5000;

const isUp = (s) => s?.state === "running";

// Summarise a bundle's containers: "3/4 up", plus whether anything is down.
function health(services) {
  const list = services || [];
  const up = list.filter(isUp).length;
  return { up, total: list.length, degraded: up < list.length };
}

function useHash() {
  const [hash, setHash] = useState(() => location.hash.slice(1));
  useEffect(() => {
    const on = () => setHash(location.hash.slice(1));
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return hash;
}

function useStored(key, initial) {
  const [v, setV] = useState(() => {
    try {
      const s = localStorage.getItem(key);
      return s === null ? initial : JSON.parse(s);
    } catch {
      return initial;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {}
  }, [key, v]);
  return [v, setV];
}

function usePoll(fn, ms) {
  useEffect(() => {
    fn();
    const t = setInterval(fn, ms);
    return () => clearInterval(t);
  }, [fn, ms]);
}

export default function App() {
  const hash = useHash();
  const [self, setSelf] = useState(null);
  const [selfErr, setSelfErr] = useState(null);
  const [gateways, setGateways] = useState([]);
  const [gwErr, setGwErr] = useState(null);
  const [projects, setProjects] = useState([]);
  const [projErr, setProjErr] = useState(null);
  const [jobsCatalog, setJobsCatalog] = useState([]);
  const [jobsDefaultProject, setJobsDefaultProject] = useState("");
  const [jobRuns, setJobRuns] = useState({});

  const refreshSelf = useCallback(async () => {
    try {
      setSelf(await api.self());
      setSelfErr(null);
    } catch (e) {
      setSelfErr(e.message);
    }
  }, []);
  usePoll(refreshSelf, SELF_POLL_MS);

  const refresh = useCallback(async () => {
    try {
      const r = await api.gateways();
      setGateways(r.gateways || []);
      setGwErr(r.error || null);
    } catch (e) {
      setGwErr(e.message);
    }
  }, []);
  usePoll(refresh, POLL_MS);

  const refreshProjects = useCallback(async () => {
    if (!self?.projectsEnabled) return;
    try {
      const r = await api.projects();
      setProjects(r.projects || []);
      setProjErr(r.error || null);
    } catch (e) {
      setProjErr(e.message);
    }
  }, [self?.projectsEnabled]);
  usePoll(refreshProjects, POLL_MS);

  useEffect(() => {
    api
      .jobs()
      .then((r) => {
        setJobsCatalog(r.jobs || []);
        setJobsDefaultProject(r.defaultProject || "");
      })
      .catch(() => {});
  }, []);
  const refreshJobRuns = useCallback(async () => {
    try {
      const r = await api.jobRuns();
      setJobRuns(r.runs || {});
    } catch {
      // Jobs page shows its own per-run errors; this is just the live poll.
    }
  }, []);
  usePoll(refreshJobRuns, POLL_MS);

  const services = useMemo(
    () =>
      (self?.services || [])
        .filter((s) => SERVICES[s])
        .map((s) => {
          // A host bundle has no dozzle of its own: link the one watching
          // the daemon it shares.
          const shared = s === "dozzle" && self.dozzlePath;
          return {
            id: s,
            ...SERVICES[s],
            path: shared ? self.dozzlePath : `${s}/`,
            desc: shared ? "Shared daemon's logs" : SERVICES[s].desc,
            status: shared ? undefined : (self.bundle || []).find((b) => b.service === SERVICES[s].compose),
          };
        }),
    [self]
  );

  // Routes: "<service>", "status", "new", "gw/<name>", "gw/<name>/info",
  // "newproj", "proj/<name>".
  const route = useMemo(() => {
    if (hash === "new") return { kind: "new" };
    if (hash === "newproj") return { kind: "newproj" };
    if (hash === "jobs") return { kind: "jobs" };
    if (hash === "status") return { kind: "status" };
    if (hash.startsWith("gw/")) {
      const [name, sub] = hash.slice(3).split("/");
      return { kind: "gw", name, info: sub === "info" };
    }
    if (hash.startsWith("proj/")) {
      return { kind: "proj", name: hash.slice(5) };
    }
    const svc = services.find((s) => s.id === hash) || services[0];
    return svc ? { kind: "svc", id: svc.id } : self ? { kind: "status" } : { kind: "loading" };
  }, [hash, services, self]);

  const current = route.kind === "gw" ? gateways.find((g) => g.name === route.name) : null;
  const currentProj = route.kind === "proj" ? projects.find((p) => p.name === route.name) : null;
  const currentSvc = route.kind === "svc" ? services.find((s) => s.id === route.id) : null;
  // A service whose container we can see but isn't running gets a status
  // panel with its logs instead of an iframe full of 502s.
  const svcDown = currentSvc && currentSvc.status && !isUp(currentSvc.status);

  // Iframes stay mounted once visited, so hopping between a child gateway
  // and a service doesn't reload the whole nested cascade.
  const [visited, setVisited] = useState({});
  const [reloads, setReloads] = useState({}); // frame key -> reload count
  const activeFrame =
    route.kind === "svc" && !svcDown
      ? `svc:${route.id}`
      : route.kind === "gw" && !route.info && current?.state === "running"
        ? `gw:${current.name}`
        : null;
  useEffect(() => {
    if (activeFrame && !visited[activeFrame]) setVisited((v) => ({ ...v, [activeFrame]: true }));
  }, [activeFrame, visited]);
  // Drop frames for gateways that no longer run.
  useEffect(() => {
    const live = new Set(gateways.filter((g) => g.state === "running").map((g) => `gw:${g.name}`));
    setVisited((v) => {
      const next = Object.fromEntries(Object.entries(v).filter(([k]) => !k.startsWith("gw:") || live.has(k)));
      return Object.keys(next).length === Object.keys(v).length ? v : next;
    });
  }, [gateways]);

  const frames = [
    ...services.map((s) => ({ key: `svc:${s.id}`, src: s.path, title: s.name })),
    ...gateways
      .filter((g) => g.state === "running")
      .map((g) => ({ key: `gw:${g.name}`, src: g.path, title: `Gateway ${g.name}` })),
  ].filter((f) => visited[f.key]);

  const popout = activeFrame ? frames.find((f) => f.key === activeFrame)?.src : null;

  useEffect(() => {
    const label =
      route.kind === "svc"
        ? SERVICES[route.id]?.name
        : route.kind === "gw"
          ? `gw/${route.name}`
          : route.kind === "proj"
            ? `proj/${route.name}`
            : route.kind === "new"
              ? "New gateway"
              : route.kind === "newproj"
                ? "New project"
                : route.kind === "jobs"
                  ? "Deployment"
                  : route.kind === "status"
                    ? "Gateway"
                    : "";
    document.title = `D8Z ${self && self.id !== "root" ? `[${self.id}] ` : ""}· ${label}`;
  }, [route, self]);

  const [collapsed, setCollapsed] = useStored(`d8z-nav-collapsed:${self?.id || "root"}`, false);

  // Root-only: which project this console is currently working on. ""
  // means "not chosen yet" (the fresh-start state - gates Deightz below);
  // "self" means explicitly this repo; anything else is a project name.
  // New gateways inherit whichever is picked; a sub-gateway's own project
  // comes from its spawn request instead, never from this picker.
  const [storedProject, setSelectedProject] = useStored(`d8z-project:${self?.id || "root"}`, "");
  const selectedProject =
    storedProject === "self" || projects.some((p) => p.name === storedProject) ? storedProject : "";

  return (
    <div className="app" title="App shell">
      <Sidebar
        self={self}
        services={services}
        gateways={gateways}
        projects={projects}
        route={route}
        collapsed={collapsed}
        onToggle={() => setCollapsed(!collapsed)}
        popout={popout}
        refresh={refresh}
        selectedProject={selectedProject}
        onSelectProject={setSelectedProject}
        onReload={activeFrame ? () => setReloads((r) => ({ ...r, [activeFrame]: (r[activeFrame] || 0) + 1 })) : null}
      />
      <main title="Main canvas — shows the selected service, gateway, or panel">
        {frames.map((f) => (
          <iframe key={`${f.key}#${reloads[f.key] || 0}`} src={f.src} title={f.title} hidden={f.key !== activeFrame} />
        ))}
        {!activeFrame && (
          <div className="panel-wrap" title="Panel area — forms and status pages that aren't an embedded sub-app">
            {selfErr && <div className="banner error">Console API unreachable: {selfErr}</div>}
            {route.kind === "new" && self && (
              <NewGateway
                self={self}
                gateways={gateways}
                projects={projects}
                defaultProject={selectedProject}
                onCreated={refresh}
              />
            )}
            {route.kind === "newproj" && self && <NewProject onCreated={refreshProjects} />}
            {route.kind === "status" && self && (
              <StatusPage
                self={self}
                gateways={gateways}
                gwErr={gwErr}
                refreshSelf={refreshSelf}
                selectedProject={selectedProject}
              />
            )}
            {route.kind === "svc" && svcDown && <ServiceDown svc={currentSvc} />}
            {route.kind === "gw" && <GatewayDetails name={route.name} gw={current} gwErr={gwErr} />}
            {route.kind === "proj" && (
              <ProjectDetails name={route.name} proj={currentProj} projErr={projErr} onChanged={refreshProjects} />
            )}
            {route.kind === "jobs" && self && (
              <JobsPage
                catalog={jobsCatalog}
                runs={jobRuns}
                onRan={refreshJobRuns}
                project={selectedProject}
                defaultProject={jobsDefaultProject}
                projectsEnabled={self.projectsEnabled}
              />
            )}
            {route.kind === "loading" && !selfErr && <p className="muted">Loading…</p>}
          </div>
        )}
      </main>
    </div>
  );
}

function Sidebar({
  self,
  services,
  gateways,
  projects,
  route,
  collapsed,
  onToggle,
  popout,
  refresh,
  selectedProject,
  onSelectProject,
  onReload,
}) {
  const [confirm, setConfirm] = useState(null);
  const [err, setErr] = useState(null);
  const own = health(self?.bundle);

  async function stop(name) {
    setConfirm(null);
    setErr(null);
    try {
      await api.stop(name);
    } catch (e) {
      setErr(`${name}: ${e.message}`);
    }
    refresh();
  }

  return (
    <nav className={collapsed ? "collapsed" : ""} title="Sidebar — navigation for this gateway">
      <header title="Sidebar header — identity badge and collapse toggle">
        <span className="title">
          D8Z
          {self && (
            <span className={`badge ${self.role}`} title={self.prefix || "/"}>
              {self.id}
            </span>
          )}
        </span>
        <button className="icon-btn" onClick={onToggle} title={collapsed ? "Expand sidebar" : "Collapse sidebar"}>
          ☰
        </button>
      </header>

      {self?.projectsEnabled && (
        <div
          className="project-picker"
          title="Which project this console is working on - gates Deightz until set, and is what new gateways spawn from by default. A sub-gateway's own project comes from its spawn request instead."
        >
          <span className="section" style={{ padding: 0, marginBottom: 4 }}>
            Project
          </span>
          <select
            value={selectedProject}
            onChange={(e) => {
              if (e.target.value === "__new__") {
                location.hash = "newproj";
                return;
              }
              onSelectProject(e.target.value);
            }}
            className={selectedProject ? "picked" : "unset"}
          >
            {!selectedProject && (
              <option value="" disabled>
                — select —
              </option>
            )}
            <option value="self">this repo</option>
            {projects.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
                {!p.hasBundle ? " (no bundle)" : ""}
              </option>
            ))}
            <option value="__new__">+ New project…</option>
          </select>
        </div>
      )}

      <ul title="Gateway entry — this gateway's own health">
        <li>
          <a href="#status" className={route.kind === "status" ? "active" : ""} title="Status of this gateway">
            <span className="ico">
              <span className={`dot ${!self ? "" : !self.dockerOK || own.degraded ? "degraded" : "running"}`} />
            </span>
            <span className="label">
              Gateway
              <small>
                {!self
                  ? "…"
                  : `${own.up}/${own.total} up${self.dockerOK ? "" : " · docker unreachable"}`}
              </small>
            </span>
          </a>
        </li>
      </ul>

      <ul title="Deployment — one-shot GitHub, Docker Hub and GCP actions">
        <li>
          <a href="#jobs" className={route.kind === "jobs" ? "active" : ""} title="GitHub, Docker Hub and GCP actions">
            <span className="ico">⚙</span>
            <span className="label">Deployment</span>
          </a>
        </li>
      </ul>

      {services.length > 0 && (
        <div className="section" title="Services section — this bundle's sub-apps (deightz, dozzle, …)">
          Services
        </div>
      )}
      <ul>
        {services.map((s) => (
          <li key={s.id}>
            <a
              href={`#${s.id}`}
              className={route.kind === "svc" && route.id === s.id ? "active" : ""}
              title={`${s.name}${s.status ? ` (${s.status.status})` : ""}`}
            >
              <span className="ico">{s.ico}</span>
              <span className="label">
                {s.name}
                <small className={s.status && !isUp(s.status) ? "error-text" : ""}>
                  {s.status && !isUp(s.status) ? s.status.status : s.desc}
                </small>
              </span>
              {s.status && <span className={`dot small ${isUp(s.status) ? "running" : s.status.state}`} />}
            </a>
          </li>
        ))}
      </ul>

      <div className="section" title="Gateways section — child gateways spawned from this one, cascaded under /gw/<name>/">
        Gateways
      </div>
      <ul>
        {gateways.map((g) => {
          const h = health(g.services);
          const busy = g.state === "starting" || g.state === "stopping";
          const dot = g.state === "running" && h.degraded ? "degraded" : g.state;
          return (
            <li key={g.name} className="gw-item">
              <a
                href={`#gw/${g.name}`}
                className={route.kind === "gw" && route.name === g.name ? "active" : ""}
                title={`${g.name}: ${g.phase || g.state}`}
              >
                <span className="ico">
                  <span className={`dot ${dot}`} />
                </span>
                <span className="label">
                  {g.name}
                  <small className={g.state === "error" || (g.state === "running" && h.degraded) ? "error-text" : ""}>
                    {busy
                      ? g.phase || g.state
                      : g.state === "error"
                        ? "failed to start"
                        : `${g.state} · ${h.up}/${h.total} up · ${g.docker}`}
                  </small>
                </span>
              </a>
              {confirm === g.name ? (
                <span className="confirm label">
                  <button className="danger" onClick={() => stop(g.name)}>
                    Stop
                  </button>
                  <button onClick={() => setConfirm(null)}>Keep</button>
                </span>
              ) : (
                <span className="row-actions label">
                  <a className="icon-btn" href={`#gw/${g.name}/info`} title={`Details for ${g.name}`}>
                    ⓘ
                  </a>
                  {g.state !== "stopping" && (
                    <button className="icon-btn stop" title={`Stop and remove ${g.name}`} onClick={() => setConfirm(g.name)}>
                      ■
                    </button>
                  )}
                </span>
              )}
            </li>
          );
        })}
        <li>
          <a href="#new" className={route.kind === "new" ? "active" : ""} title="New gateway">
            <span className="ico">＋</span>
            <span className="label">New gateway</span>
          </a>
        </li>
      </ul>

      {err && <div className="banner error label">{err}</div>}

      <footer title="Sidebar footer — reload the current frame or pop it out to a new tab">
        {onReload && (
          <button className="link-btn" onClick={onReload} title="Reload the current page">
            Reload ↻
          </button>
        )}
        {popout && (
          <a href={popout} target="_blank" rel="noopener">
            Open in new tab ↗
          </a>
        )}
      </footer>
    </nav>
  );
}

// LogView fetches text and keeps it fresh while `live`; sticks to the
// bottom unless the user has scrolled up to read.
function LogView({ load, live = true, empty = "No output." }) {
  const [text, setText] = useState(null);
  const [err, setErr] = useState(null);
  const pre = useRef(null);
  const stick = useRef(true);

  const fetchIt = useCallback(async () => {
    try {
      setText(await load());
      setErr(null);
    } catch (e) {
      setErr(e.message);
    }
  }, [load]);

  useEffect(() => {
    fetchIt();
    if (!live) return;
    const t = setInterval(fetchIt, 2500);
    return () => clearInterval(t);
  }, [fetchIt, live]);

  useEffect(() => {
    if (pre.current && stick.current) pre.current.scrollTop = pre.current.scrollHeight;
  }, [text]);

  if (err) return <div className="banner error">{err}</div>;
  if (text === null) return <p className="muted">Loading…</p>;
  return (
    <pre
      ref={pre}
      className="log"
      onScroll={(e) => {
        const el = e.currentTarget;
        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
      }}
    >
      {text || empty}
    </pre>
  );
}

// ServicesTable lists a bundle's containers with expandable logs. `actions`,
// if given, renders extra controls (e.g. start/stop) into a row's head.
function ServicesTable({ services, logsFor, actions }) {
  const [open, setOpen] = useState(null);
  if (!services || services.length === 0) return <p className="muted">No containers yet.</p>;
  return (
    <div className="svc-table">
      {services.map((s) => (
        <div key={s.container} className={`svc-row ${open === s.service ? "open" : ""}`}>
          <div className="svc-head">
            <span className={`dot ${s.state === "absent" ? "" : isUp(s) ? (s.status.includes("unhealthy") ? "degraded" : "running") : s.state}`} />
            <code>{s.service}</code>
            <span className={`svc-status ${isUp(s) ? "muted" : "error-text"}`}>{s.status}</span>
            {actions && actions(s)}
            {s.state !== "absent" && (
              <button className="link-btn" onClick={() => setOpen(open === s.service ? null : s.service)}>
                {open === s.service ? "Hide logs" : "Logs"}
              </button>
            )}
          </div>
          {open === s.service && <LogView load={logsFor(s.service)} live={isUp(s) || s.state === "restarting"} />}
        </div>
      ))}
    </div>
  );
}

function StatusPage({ self, gateways, gwErr, refreshSelf, selectedProject }) {
  const logsFor = useCallback((svc) => () => api.serviceLogs(svc), []);
  const [busy, setBusy] = useState(null);
  const [err, setErr] = useState(null);

  const localCatalog = useMemo(
    () => Object.fromEntries((self.localServices || []).map((s) => [s.name, s])),
    [self.localServices]
  );
  const localOps = self.localOps || {};
  const bundleRows = useMemo(() => {
    const present = new Set((self.bundle || []).map((s) => s.service));
    const missing = Object.keys(localCatalog)
      .filter((n) => !present.has(n))
      .map((n) => ({ service: n, container: n, state: "absent", status: "not started" }));
    return [...(self.bundle || []), ...missing];
  }, [self.bundle, localCatalog]);

  async function toggleLocal(svc, running) {
    setErr(null);
    setBusy(svc);
    try {
      await (running ? api.stopLocal(svc) : api.startLocal(svc, selectedProject));
    } catch (e) {
      setErr(`${svc}: ${e.message}`);
    }
    setBusy(null);
    refreshSelf();
  }

  const localActions = (s) => {
    const svc = localCatalog[s.service];
    if (!svc) return null;
    const op = localOps[s.service];
    const opBusy = op && (op.state === "starting" || op.state === "stopping");
    const running = isUp(s);
    // Fresh-start guard: Deightz opens whichever project is picked up top
    // (DEFAULT_WORKSPACE), so starting it before anything's picked just
    // opens the wrong thing. Only blocks the initial Start, root only
    // (self.projectsEnabled is already root-only), and never once running.
    if (s.service === "deightz" && self.projectsEnabled && !selectedProject && !running) {
      return (
        <span className="svc-status error-text" title="Pick a project in the sidebar first">
          select a project ↑
        </span>
      );
    }
    return (
      <button
        className={`link-btn ${running ? "stop" : ""}`}
        disabled={busy === s.service || opBusy}
        onClick={() => toggleLocal(s.service, running)}
        title={op?.error || ""}
      >
        {opBusy ? op.phase || op.state : running ? "Stop" : "Start"}
      </button>
    );
  };

  return (
    <div className="panel wide">
      <h2>
        Gateway <code>{self.id}</code> <small className="muted">{self.role}</small>
      </h2>
      <dl className="facts">
        <dt>Mounted at</dt>
        <dd>
          <code>{self.prefix || ""}/</code>
        </dd>
        <dt>Runs on</dt>
        <dd>
          {self.isolated ? "its own dind sidecar (isolated bundle)" : "a shared docker daemon"}{" "}
          {self.dockerOK ? (
            <span className="muted">(docker {self.docker})</span>
          ) : (
            <span className="error-text">unreachable: {self.dockerError}</span>
          )}
        </dd>
      </dl>

      <h3>This bundle</h3>
      {self.bundleError && <div className="banner error">{self.bundleError}</div>}
      {err && <div className="banner error">{err}</div>}
      <ServicesTable services={bundleRows} logsFor={logsFor} actions={localActions} />

      <h3>Child gateways</h3>
      {gwErr && <div className="banner error">{gwErr}</div>}
      {gateways.length === 0 ? (
        <p className="muted">
          None. <a href="#new">Start one →</a>
        </p>
      ) : (
        <div className="svc-table">
          {gateways.map((g) => {
            const h = health(g.services);
            return (
              <div key={g.name} className="svc-row">
                <div className="svc-head">
                  <span className={`dot ${g.state === "running" && h.degraded ? "degraded" : g.state}`} />
                  <a href={`#gw/${g.name}/info`}>
                    <code>gw/{g.name}</code>
                  </a>
                  <span className={`svc-status ${g.state === "error" || h.degraded ? "error-text" : "muted"}`}>
                    {g.phase || g.error || `${g.state} · ${h.up}/${h.total} up`}
                  </span>
                  <span className="muted">{g.docker}</span>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function ServiceDown({ svc }) {
  const load = useCallback(() => api.serviceLogs(svc.compose), [svc.compose]);
  return (
    <div className="panel wide">
      <h2>
        <span className={`dot ${svc.status.state}`} /> {svc.name} <small className="muted">is not running</small>
      </h2>
      <p className="error-text">{svc.status.status}</p>
      <p className="muted">
        Container <code>{svc.status.container}</code>. It restarts automatically unless stopped; this page switches to the
        app once it's up.
      </p>
      <h3>Logs</h3>
      <LogView load={load} live={svc.status.state === "restarting"} />
    </div>
  );
}

function GatewayDetails({ name, gw, gwErr }) {
  const busy = gw?.state === "starting" || gw?.state === "stopping";
  const loadSpawn = useCallback(() => api.log(name), [name]);
  const logsFor = useCallback((svc) => () => api.childServiceLogs(name, svc), [name]);

  if (!gw) {
    return (
      <div className="panel">
        <h2>gw/{name}</h2>
        <p className="muted">{gwErr ? `Can't list gateways: ${gwErr}` : "This gateway doesn't exist (any more)."}</p>
        <a href="#new">Start a new gateway →</a>
      </div>
    );
  }
  return (
    <div className="panel wide">
      <h2>
        <span className={`dot ${gw.state}`} /> gw/{gw.name} <small className="muted">{gw.state}</small>
        {gw.state === "running" && (
          <a className="h-link" href={`#gw/${gw.name}`}>
            Open console →
          </a>
        )}
      </h2>
      {gw.phase && (
        <p className="phase">
          {busy && <span className="spinner" />} {gw.phase}
        </p>
      )}
      {gw.error && <div className="banner error">{gw.error}</div>}
      <dl className="facts">
        <dt>Project</dt>
        <dd>
          <code>{gw.project}</code>
        </dd>
        <dt>Source</dt>
        <dd>{gw.srcProject ? <code>proj/{gw.srcProject}</code> : "this repo"}</dd>
        <dt>Prefix</dt>
        <dd>
          <code>{gw.prefix}/</code>
        </dd>
        <dt>Docker</dt>
        <dd>
          {gw.docker === "dind"
            ? "dind: the whole bundle runs inside its sidecar; only the sidecar is on our daemon"
            : "host: the bundle runs on our daemon"}
        </dd>
        <dt>Profiles</dt>
        <dd>{(gw.profiles || []).join(", ") || "none"}</dd>
        {gw.upstream && (
          <>
            <dt>Upstream</dt>
            <dd>
              <code>{gw.upstream}</code>
            </dd>
          </>
        )}
      </dl>

      <h3>Containers</h3>
      <ServicesTable services={gw.services} logsFor={logsFor} />

      <h3>Start / stop log</h3>
      <LogView load={loadSpawn} live={busy} empty="Nothing logged." />
    </div>
  );
}

function NewGateway({ self, gateways, projects, defaultProject, onCreated }) {
  const uiProfiles = self.profiles || [];
  const bundleProjects = (projects || []).filter((p) => p.hasBundle);
  const [name, setName] = useState("");
  const [picked, setPicked] = useState(() => new Set(["deightz"]));
  const [docker, setDocker] = useState("host");
  // Inherits the sidebar's current project pick, if it's actually spawnable.
  const [project, setProject] = useState(() =>
    bundleProjects.some((p) => p.name === defaultProject) ? defaultProject : ""
  );
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  // One dozzle per docker daemon: a dind bundle is a new daemon and always
  // gets one; a host bundle shares ours (and our dozzle). The server
  // enforces the same rule.
  const fixed = (n) => (n === "dozzle" ? docker === "dind" : null);

  const nameOk = /^[a-z][a-z0-9]{0,15}$/.test(name);
  const taken = gateways.some((g) => g.name === name);

  async function submit(e) {
    e.preventDefault();
    if (!nameOk || taken) return;
    setBusy(true);
    setErr(null);
    try {
      await api.spawn({ name, profiles: [...picked].filter((n) => fixed(n) === null), docker, project });
      await onCreated();
      location.hash = `gw/${name}`;
    } catch (e) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  const toggle = (n) =>
    setPicked((s) => {
      const next = new Set(s);
      next.has(n) ? next.delete(n) : next.add(n);
      return next;
    });

  return (
    <form className="panel" onSubmit={submit}>
      <h2>New gateway</h2>
      <p className="muted">
        Starts an isolated bundle of services behind its own Traefik, mounted here at{" "}
        <code>
          {self.prefix || ""}/gw/{name || "<name>"}/
        </code>
        . Only the two Traefiks share a network.
      </p>
      {!self.dockerOK && (
        <div className="banner error">
          This gateway can't reach Docker: {self.dockerError}
        </div>
      )}

      <label className="field">
        <span>Name</span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value.toLowerCase())}
          placeholder="e.g. sandbox"
          autoFocus
          maxLength={16}
        />
        <small className={name && (!nameOk || taken) ? "error-text" : "muted"}>
          {taken ? "Already exists." : "Lowercase letters and digits, starting with a letter."}
        </small>
      </label>

      {bundleProjects.length > 0 && (
        <label className="field">
          <span>Project</span>
          <select value={project} onChange={(e) => setProject(e.target.value)}>
            <option value="">this repo</option>
            {bundleProjects.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
          <small className="muted">
            Which source tree's own dev/traefik/bundle/compose.yaml to spawn.
            {defaultProject && defaultProject !== "self" && !bundleProjects.some((p) => p.name === defaultProject)
              ? ` Sidebar has "${defaultProject}" selected, but it has no bundle - defaulted to this repo.`
              : " Inherited from the sidebar's project picker; change it here just for this spawn."}
          </small>
        </label>
      )}

      <fieldset className="field">
        <legend>Services</legend>
        {uiProfiles.map((p) => (
          <label key={p.name} className="check">
            <input
              type="checkbox"
              checked={fixed(p.name) ?? picked.has(p.name)}
              disabled={fixed(p.name) !== null}
              onChange={() => toggle(p.name)}
            />
            {p.label} <code>{p.name}</code>
            {p.name === "dozzle" && (
              <small className="muted">
                {docker === "dind" ? " (one per docker daemon: always on for dind)" : " (shares this daemon's dozzle)"}
              </small>
            )}
          </label>
        ))}
      </fieldset>

      <fieldset className="field">
        <legend>Where the bundle runs</legend>
        <label className="check">
          <input type="radio" name="docker" checked={docker === "host"} onChange={() => setDocker("host")} />
          <span>
            <b>host</b>: on this gateway's daemon. Starts fast and reuses images, but its docker-using services (dozzle,
            deightz) can see sibling bundles.
          </span>
        </label>
        <label className="check">
          <input type="radio" name="docker" checked={docker === "dind"} onChange={() => setDocker("dind")} />
          <span>
            <b>dind</b>: inside its own dind sidecar; only the sidecar links up to this gateway. Fully isolated: dozzle
            shows exactly the bundle and its children. Images are copied in first, which takes a while for big ones.
          </span>
        </label>
      </fieldset>

      {err && <div className="banner error">{err}</div>}
      <div className="actions">
        <button className="primary" disabled={busy || !nameOk || taken || !self.dockerOK}>
          {busy ? "Starting…" : "Start gateway"}
        </button>
      </div>
    </form>
  );
}

function ProjectDetails({ name, proj, projErr, onChanged }) {
  const logsFor = useCallback((svc) => () => api.projectLog(name), [name]);
  const loadLog = useCallback(() => api.projectLog(name), [name]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  if (!proj) {
    return (
      <div className="panel">
        <h2>proj/{name}</h2>
        <p className="muted">{projErr ? `Can't list projects: ${projErr}` : "This project doesn't exist (any more)."}</p>
        <a href="#newproj">Provision a new project →</a>
      </div>
    );
  }

  const h = health(proj.services);
  const running = h.total > 0;
  const opBusy = proj.state === "starting" || proj.state === "stopping";

  async function toggle() {
    setErr(null);
    setBusy(true);
    try {
      await (running ? api.stopProject(name) : api.startProject(name));
    } catch (e) {
      setErr(e.message);
    }
    setBusy(false);
    onChanged();
  }

  return (
    <div className="panel wide">
      <h2>
        <span className={`dot ${opBusy ? proj.state : running ? (h.degraded ? "degraded" : "running") : ""}`} />{" "}
        proj/{proj.name}
      </h2>
      {proj.phase && (
        <p className="phase">
          {opBusy && <span className="spinner" />} {proj.phase}
        </p>
      )}
      {proj.error && <div className="banner error">{proj.error}</div>}
      {err && <div className="banner error">{err}</div>}
      <dl className="facts">
        <dt>Path</dt>
        <dd>
          <code>~/projects/{proj.name}</code>
        </dd>
        <dt>Has</dt>
        <dd>
          {proj.hasCompose ? "docker-compose.yaml" : "no docker-compose.yaml"}
          {proj.hasBundle && ", dev/traefik/bundle/compose.yaml (spawnable as a gateway)"}
        </dd>
      </dl>

      {proj.hasCompose && (
        <div className="actions">
          <button className="primary" disabled={busy || opBusy} onClick={toggle}>
            {opBusy ? proj.phase || proj.state : running ? "Stop" : "Start"}
          </button>
          {proj.hasBundle && (
            <a className="h-link" href={`#new`}>
              Spawn a gateway from this project →
            </a>
          )}
        </div>
      )}

      <h3>Containers</h3>
      <ServicesTable services={proj.services} logsFor={logsFor} />

      <h3>Start / stop log</h3>
      <LogView load={loadLog} live={opBusy} empty="Nothing logged." />
    </div>
  );
}

function NewProject({ onCreated }) {
  const [name, setName] = useState("");
  const [kind, setKind] = useState("self");
  const [url, setUrl] = useState("");
  const [ref, setRef] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const nameOk = /^[a-z][a-z0-9-]{0,63}$/.test(name);
  const urlOk = kind === "self" || url.trim() !== "";

  async function submit(e) {
    e.preventDefault();
    if (!nameOk || !urlOk) return;
    setBusy(true);
    setErr(null);
    try {
      const source = kind === "self" ? { kind } : { kind, url: url.trim(), ref: ref.trim() };
      await api.createProject({ name, source });
      await onCreated();
      location.hash = `proj/${name}`;
    } catch (e) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="panel" onSubmit={submit}>
      <h2>New project</h2>
      <p className="muted">
        Provisions <code>~/projects/{name || "<name>"}</code> on the host. A directory that already exists there is
        never overwritten - this only ever creates a new one.
      </p>

      <label className="field">
        <span>Name</span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value.toLowerCase())}
          placeholder="e.g. d8z-cloud-env-next"
          autoFocus
          maxLength={64}
        />
        <small className={name && !nameOk ? "error-text" : "muted"}>
          Lowercase letters, digits and dashes, starting with a letter.
        </small>
      </label>

      <fieldset className="field">
        <legend>Source</legend>
        <label className="check">
          <input type="radio" name="kind" checked={kind === "self"} onChange={() => setKind("self")} />
          <span>
            <b>This repo</b>: a plain copy of this image's own checkout - the quickest way to get a second, independent
            copy of this same stack to spawn as a gateway.
          </span>
        </label>
        <label className="check">
          <input type="radio" name="kind" checked={kind === "git"} onChange={() => setKind("git")} />
          <span>
            <b>Git clone</b>: a shallow clone of a repository - e.g. to pull the latest version of this project, or to
            bring in an unrelated one.
          </span>
        </label>
        {kind === "git" && (
          <>
            <label className="field">
              <span>URL</span>
              <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://github.com/org/repo.git" />
            </label>
            <label className="field">
              <span>Ref (optional)</span>
              <input value={ref} onChange={(e) => setRef(e.target.value)} placeholder="branch, tag or empty for default" />
            </label>
          </>
        )}
      </fieldset>

      {err && <div className="banner error">{err}</div>}
      <div className="actions">
        <button className="primary" disabled={busy || !nameOk || !urlOk}>
          {busy ? "Provisioning…" : "Create project"}
        </button>
      </div>
    </form>
  );
}

const JOB_GROUPS = { github: "GitHub", docker: "Docker Hub", cloud: "GCP" };

function JobsPage({ catalog, runs, onRan, project, defaultProject, projectsEnabled }) {
  const groups = useMemo(() => {
    const g = {};
    for (const j of catalog || []) (g[j.group] ||= []).push(j);
    return g;
  }, [catalog]);

  if (!catalog || catalog.length === 0) return <p className="muted">Loading…</p>;

  // "" / "self" both mean the .env's own PROJECT on the server side.
  const target = project && project !== "self" ? project : defaultProject;

  return (
    <div className="panel wide">
      <h2>Deployment</h2>
      <p className="muted">
        One-shot actions against GitHub, Docker Hub and GCP. GitHub and Docker Hub run directly in the gateway; GCP
        runs in the <code>dev/cloud</code> container. Dangerous ones (a push, a repo or branch write, a billable
        instance) ask you to confirm first.
      </p>
      <dl className="facts">
        <dt>Project</dt>
        <dd>
          {target ? <code>{target}</code> : <span className="error-text">none - pick one in the sidebar</span>}{" "}
          <span className="muted">
            {projectsEnabled ? "(the sidebar's selection; GitHub repo defaults to its git origin)" : "(child gateway: no project files)"}
          </span>
        </dd>
      </dl>
      {Object.entries(groups).map(([group, items]) => (
        <div key={group}>
          <h3>{JOB_GROUPS[group] || group}</h3>
          <div className="svc-table">
            {items.map((j) => (
              <JobRow key={`${j.group}/${j.name}`} job={j} run={runs[`${j.group}/${j.name}`]} onRan={onRan} project={project} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function JobRow({ job, run, onRan, project }) {
  const id = `${job.group}/${job.name}`;
  const [args, setArgs] = useState({});
  const [confirm, setConfirm] = useState(false);
  const [open, setOpen] = useState(false);
  const [err, setErr] = useState(null);

  const opBusy = run?.state === "starting";
  const argsOk = (job.args || []).every((a) => !a.required || (args[a.name] || "").trim() || a.default);

  async function fire() {
    setConfirm(false);
    setErr(null);
    try {
      // Blank args are left out: the server fills in their defaults.
      const sent = Object.fromEntries(
        Object.entries(args)
          .map(([k, v]) => [k, v.trim()])
          .filter(([, v]) => v)
      );
      await api.runJob(job.group, job.name, project, sent);
      setOpen(true);
      onRan();
    } catch (e) {
      setErr(e.message);
    }
  }

  const dot = !run ? null : opBusy ? "starting" : run.state === "error" ? "error" : "running";

  return (
    <div className="svc-row">
      <div className="svc-head">
        {dot && <span className={`dot ${dot}`} />}
        <code>{job.name}</code>
        <span className="svc-status muted">{job.label}</span>
        {(job.args || []).map((a) => {
          const ph = a.default ? `${a.label || a.name}: ${a.default}` : a.label || a.name;
          return (
            <input
              key={a.name}
              value={args[a.name] || ""}
              placeholder={ph}
              title={ph}
              size={Math.min(28, Math.max(10, ph.length))}
              onChange={(e) => setArgs((x) => ({ ...x, [a.name]: e.target.value }))}
            />
          );
        })}
        {confirm ? (
          <span className="confirm label">
            <button className="danger" onClick={fire}>
              Really run
            </button>
            <button onClick={() => setConfirm(false)}>Cancel</button>
          </span>
        ) : (
          <button
            className={`link-btn ${job.dangerous ? "stop" : ""}`}
            disabled={opBusy || !argsOk}
            onClick={() => (job.dangerous ? setConfirm(true) : fire())}
          >
            {opBusy ? run.phase || run.state : "Run"}
          </button>
        )}
        {run && (
          <button className="link-btn" onClick={() => setOpen((o) => !o)}>
            {open ? "Hide log" : "Log"}
          </button>
        )}
      </div>
      {run?.error && <div className="banner error">{run.error}</div>}
      {err && <div className="banner error">{err}</div>}
      {open && run && <LogView load={() => api.jobLog(id)} live={opBusy} empty="Nothing logged." />}
    </div>
  );
}

