import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { WorkspacesPage } from "./Workspaces.jsx";
import { StopConfirm } from "./StopConfirm.jsx";
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

export const isUp = (s) => s?.state === "running";

// Summarise a bundle's containers: "3/4 up", plus whether anything is down.
export function health(services) {
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
  const [wsInfo, setWsInfo] = useState(null);
  const [wsErr, setWsErr] = useState(null);
  const [jobsCatalog, setJobsCatalog] = useState([]);
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

  const refreshWorkspaces = useCallback(async () => {
    try {
      setWsInfo(await api.workspaces());
      setWsErr(null);
    } catch (e) {
      setWsErr(e.message);
    }
  }, []);
  usePoll(refreshWorkspaces, POLL_MS);

  useEffect(() => {
    api
      .jobs()
      .then((r) => setJobsCatalog(r.jobs || []))
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
        // Only services this bundle actually has: the root starts with just
        // traefik + dozzle, editors live in the workspace gateways.
        .filter(
          (s) =>
            (s === "dozzle" && self.dozzlePath) ||
            self.bundleError ||
            (self.bundle || []).some((b) => b.service === SERVICES[s].compose)
        )
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

  // Routes: "<service>", "status", "workspaces", "jobs", "gw/<name>",
  // "gw/<name>/info". ("new" - the old New gateway page - now lands on
  // Workspaces, where gateways are started.)
  const route = useMemo(() => {
    if (hash === "workspaces" || hash === "new") return { kind: "workspaces" };
    if (hash === "jobs") return { kind: "jobs" };
    if (hash === "status") return { kind: "status" };
    if (hash.startsWith("gw/")) {
      const [name, sub] = hash.slice(3).split("/");
      return { kind: "gw", name, info: sub === "info" };
    }
    const svc = services.find((s) => s.id === hash) || services[0];
    return svc ? { kind: "svc", id: svc.id } : self ? { kind: "status" } : { kind: "loading" };
  }, [hash, services, self]);

  const current = route.kind === "gw" ? gateways.find((g) => g.name === route.name) : null;
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
          : route.kind === "workspaces"
            ? "Workspaces"
            : route.kind === "jobs"
                  ? "Deployment"
                  : route.kind === "status"
                    ? "Gateway"
                    : "";
    document.title = `D8Z ${self && self.id !== "root" ? `[${self.id}] ` : ""}· ${label}`;
  }, [route, self]);

  const [collapsed, setCollapsed] = useStored(`d8z-nav-collapsed:${self?.id || "root"}`, false);

  return (
    <div className="app" title="App shell">
      <Sidebar
        self={self}
        services={services}
        gateways={gateways}
        wsInfo={wsInfo}
        route={route}
        collapsed={collapsed}
        onToggle={() => setCollapsed(!collapsed)}
        popout={popout}
        refresh={refresh}
        onReload={activeFrame ? () => setReloads((r) => ({ ...r, [activeFrame]: (r[activeFrame] || 0) + 1 })) : null}
      />
      <main title="Main canvas — shows the selected service, gateway, or panel">
        {frames.map((f) => (
          <iframe key={`${f.key}#${reloads[f.key] || 0}`} src={f.src} title={f.title} hidden={f.key !== activeFrame} />
        ))}
        {!activeFrame && (
          <div className="panel-wrap" title="Panel area — forms and status pages that aren't an embedded sub-app">
            {selfErr && <div className="banner error">Console API unreachable: {selfErr}</div>}
            {route.kind === "workspaces" && self && (
              <WorkspacesPage
                info={wsInfo}
                err={wsErr}
                onChanged={() => {
                  refreshWorkspaces();
                  refresh();
                }}
                catalog={jobsCatalog}
                runs={jobRuns}
                onRan={refreshJobRuns}
                self={self}
              />
            )}
            {route.kind === "status" && self && (
              <StatusPage
                self={self}
                gateways={gateways}
                gwErr={gwErr}
                refreshSelf={refreshSelf}
              />
            )}
            {route.kind === "svc" && svcDown && <ServiceDown svc={currentSvc} />}
            {route.kind === "gw" && <GatewayDetails name={route.name} gw={current} gwErr={gwErr} />}
            {route.kind === "jobs" && self && (
              <JobsPage
                catalog={jobsCatalog}
                runs={jobRuns}
                onRan={refreshJobRuns}
                workspace={self.workspace}
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
  wsInfo,
  route,
  collapsed,
  onToggle,
  popout,
  refresh,
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

      {self?.workspace && (
      <ul title="This gateway's workspace — pull, diff, push and pull requests">
        <li>
          <a
            href="#workspaces"
            className={route.kind === "workspaces" ? "active" : ""}
            title={
              self?.workspace
                ? "This gateway's workspace: pull, diff, push and pull requests"
                : "Add and remove workspaces, and start a gateway for each"
            }
          >
            <span className="ico">▤</span>
            <span className="label">
              {self?.workspace ? "Workspace" : "Workspaces"}
              <small>
                {self?.workspace
                  ? self.workspace.name
                  : wsInfo?.workspaces
                    ? `${wsInfo.workspaces.length} configured · ${wsInfo.workspaces.filter((w) => w.gw).length} with gateway`
                    : "…"}
              </small>
            </span>
          </a>
        </li>
      </ul>
      )}

      <ul title="Deployment — one-shot Docker Hub and GCP actions">
        <li>
          <a href="#jobs" className={route.kind === "jobs" ? "active" : ""} title="Docker Hub and GCP actions">
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

      <div
        className="section"
        title="Workspaces section — each workspace's own gateway (its editor and repository tools), cascaded under /gw/<name>/"
      >
        Workspaces
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
                title={`${g.workspace ? `${g.workspace} (gw/${g.name})` : `gw/${g.name}`}: ${g.phase || g.state}`}
              >
                <span className="ico">
                  <span className={`dot ${dot}`} />
                </span>
                <span className="label">
                  {g.workspace || g.name}
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
                <div className="confirm label">
                  <StopConfirm gw={g} compact onStop={() => stop(g.name)} onKeep={() => setConfirm(null)} />
                </div>
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
        {!self?.workspace && (
          <li>
            <a
              href="#workspaces"
              className={route.kind === "workspaces" ? "active" : ""}
              title="Add, edit and remove workspaces; start or stop each one's gateway"
            >
              <span className="ico">▤</span>
              <span className="label">
                Manage workspaces
                <small>
                  {wsInfo?.workspaces
                    ? `${wsInfo.workspaces.length} configured · ${wsInfo.workspaces.filter((w) => w.gw).length} running`
                    : "…"}
                </small>
              </span>
            </a>
          </li>
        )}
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
export function LogView({ load, live = true, empty = "No output." }) {
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

function StatusPage({ self, gateways, gwErr, refreshSelf }) {
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
      await (running ? api.stopLocal(svc) : api.startLocal(svc));
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
    // Editors belong to workspaces: on a manager, Deightz is started per
    // workspace (Workspaces tab), not here.
    if (s.service === "deightz" && !self.workspace && !running && s.state === "absent") {
      return (
        <a className="svc-status" href="#workspaces" title="Each workspace gets its own gateway with a Deightz">
          start one per workspace →
        </a>
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
        {self.workspace && (
          <>
            <dt>Workspace</dt>
            <dd>
              <a href="#workspaces">
                <code>{self.workspace.name}</code>
              </a>{" "}
              <span className="muted">({self.workspace.source})</span>
            </dd>
          </>
        )}
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

      <h3>Workspace gateways</h3>
      {gwErr && <div className="banner error">{gwErr}</div>}
      {gateways.length === 0 ? (
        <p className="muted">
          None. <a href="#workspaces">Start one from Workspaces →</a>
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
                    <code>{g.workspace || `gw/${g.name}`}</code>
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
        <a href="#workspaces">Workspaces →</a>
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
        <dt>Workspace</dt>
        <dd>{gw.workspace ? <a href="#workspaces"><code>{gw.workspace}</code></a> : <span className="muted">none</span>}</dd>
        <dt>Bundle from</dt>
        <dd>{gw.srcProject ? <code>~/projects/{gw.srcProject}</code> : "this repo"}</dd>
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

const JOB_GROUPS = { github: "GitHub", docker: "Docker Hub", cloud: "GCP" };

function JobsPage({ catalog, runs, onRan, workspace }) {
  // Repository work (github/*) lives on the Workspace tab of a workspace's
  // own gateway; this page keeps the deployment targets.
  const groups = useMemo(() => {
    const g = {};
    for (const j of catalog || []) if (!j.workspace) (g[j.group] ||= []).push(j);
    return g;
  }, [catalog]);

  if (!catalog || catalog.length === 0) return <p className="muted">Loading…</p>;

  return (
    <div className="panel wide">
      <h2>Deployment</h2>
      <p className="muted">
        One-shot actions against Docker Hub and GCP. Docker Hub runs directly in the gateway; GCP runs in the{" "}
        <code>dev/cloud</code> container. Dangerous ones (a push, a repo write, a billable instance) ask you to confirm
        first. GitHub pull / diff / push / pull requests moved to <a href="#workspaces">{workspace ? "Workspace" : "Workspaces"}</a>.
      </p>
      <dl className="facts">
        <dt>Builds from</dt>
        <dd>
          {workspace ? (
            <>
              workspace <code>{workspace.name}</code>
            </>
          ) : (
            <span className="muted">this gateway's own project (.env PROJECT)</span>
          )}
        </dd>
      </dl>
      {Object.entries(groups).map(([group, items]) => (
        <div key={group}>
          <h3>{JOB_GROUPS[group] || group}</h3>
          <div className="svc-table">
            {items.map((j) => (
              <JobRow key={`${j.group}/${j.name}`} job={j} run={runs[`${j.group}/${j.name}`]} onRan={onRan} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

export function JobRow({ job, run, onRan, project = "" }) {
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

