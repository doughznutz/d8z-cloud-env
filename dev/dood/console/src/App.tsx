import type { Dispatch, FormEvent, SetStateAction } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AdHocLaunchPage,
  OwnWorkspacePanel,
  WorkspaceAddPage,
  WorkspaceDetails,
  WorkspacePending,
  WorkspaceStartLog,
} from "./Workspaces";
import { StopConfirm } from "./StopConfirm";
import { DeploymentPage } from "./Deployment";
import { api } from "./api";
import { EntityCard, EntityRow, EntitySection, Panel } from "./design/entity";
import type { Fact } from "./design/entity";
import { Messenger, ParentAndSiblings } from "./Messenger";
import { isUp, health } from "./health";
import { LogView } from "./LogView";
import { GatewayLivePanel } from "./GatewayLivePanel";
import { RowMenu } from "./RowMenu";
import type { Gateway, Job, JobRun, Request, Route, Self, ServiceStatus, WorkspaceInfo } from "./types";

// Sub-apps this console knows how to show; which ones appear comes from
// the gateway's own service list (api/self). `compose` is the bundle
// service backing it, for health and logs.
const SERVICES: Record<string, { ico: string; name: string; desc: string; compose: string }> = {
  deightz: { ico: "⌨", name: "Deightz", desc: "Code-server IDE", compose: "deightz" },
  og: { ico: "✱", name: "OG", desc: "Ollama gateway", compose: "og" },
  gopher: { ico: "✓", name: "Gopher", desc: "Tests & docs", compose: "gopher" },
  dozzle: { ico: "≡", name: "Dozzle", desc: "Container logs", compose: "dozzle" },
};

const POLL_MS = 3000;
const SELF_POLL_MS = 5000;

/** A sidebar nav entry for one of this bundle's sub-apps. */
interface ComputedService {
  id: string;
  ico: string;
  name: string;
  desc: string;
  compose: string;
  path: string;
  status?: ServiceStatus;
}

function useHash(): string {
  const [hash, setHash] = useState(() => location.hash.slice(1));
  useEffect(() => {
    const on = () => setHash(location.hash.slice(1));
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return hash;
}

function useStored<T>(key: string, initial: T): [T, Dispatch<SetStateAction<T>>] {
  const [v, setV] = useState<T>(() => {
    try {
      const s = localStorage.getItem(key);
      return s === null ? initial : (JSON.parse(s) as T);
    } catch {
      return initial;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      // Storage may be unavailable (private browsing, quota); persistence is best-effort.
    }
  }, [key, v]);
  return [v, setV];
}

function usePoll(fn: () => void | Promise<void>, ms: number): void {
  useEffect(() => {
    void fn();
    const t = setInterval(fn, ms);
    return () => clearInterval(t);
  }, [fn, ms]);
}

/** The console's top-level app shell: sidebar navigation plus the active panel or embedded sub-app. */
export default function App() {
  const hash = useHash();
  const [self, setSelf] = useState<Self | null>(null);
  const [selfErr, setSelfErr] = useState<string | null>(null);
  const [gateways, setGateways] = useState<Gateway[]>([]);
  const [gwErr, setGwErr] = useState<string | null>(null);
  const [wsInfo, setWsInfo] = useState<WorkspaceInfo | null>(null);
  const [wsErr, setWsErr] = useState<string | null>(null);
  const [jobsCatalog, setJobsCatalog] = useState<Job[]>([]);
  const [jobRuns, setJobRuns] = useState<Record<string, JobRun>>({});

  const refreshSelf = useCallback(async () => {
    try {
      setSelf(await api.self());
      setSelfErr(null);
    } catch (e) {
      setSelfErr(e instanceof Error ? e.message : String(e));
    }
  }, []);
  usePoll(refreshSelf, SELF_POLL_MS);

  const refresh = useCallback(async () => {
    try {
      const r = await api.gateways();
      setGateways(r.gateways || []);
      setGwErr(r.error || null);
    } catch (e) {
      setGwErr(e instanceof Error ? e.message : String(e));
    }
  }, []);
  usePoll(refresh, POLL_MS);

  const refreshWorkspaces = useCallback(async () => {
    try {
      setWsInfo(await api.workspaces());
      setWsErr(null);
    } catch (e) {
      setWsErr(e instanceof Error ? e.message : String(e));
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

  const services = useMemo<ComputedService[]>(
    () =>
      (self?.services || [])
        .filter((s) => SERVICES[s])
        // Only services this bundle actually has: the root profile starts
        // with just dood; deightz comes with the workspace profile (or a
        // workspace gateway), og/gopher/dozzle on demand from the Control tab.
        .filter(
          (s) =>
            (s === "dozzle" && self?.dozzlePath) ||
            self?.bundleError ||
            (self?.bundle || []).some((b) => b.service === SERVICES[s].compose)
        )
        .map((s) => {
          // A host bundle has no dozzle of its own: link the one watching
          // the daemon it shares.
          const shared = s === "dozzle" && self?.dozzlePath;
          return {
            id: s,
            ...SERVICES[s],
            path: shared ? self.dozzlePath! : `${s}/`,
            desc: shared ? "Shared daemon's logs" : SERVICES[s].desc,
            status: shared ? undefined : (self?.bundle || []).find((b) => b.service === SERVICES[s].compose),
          };
        }),
    [self]
  );

  // Routes: "<service>", "control" (old "status"), "jobs", "ws-new", "gw-new",
  // "ws/<name>", "ws/<name>/settings|edit|log", "gw/<name>", "gw/<name>/info".
  const route = useMemo<Route>(() => {
    if (hash === "jobs") return { kind: "jobs" };
    if (hash === "control" || hash === "status") return { kind: "control" };
    if (hash === "ws-new") return { kind: "ws-new" };
    if (hash === "gw-new" || hash === "new") return { kind: "gw-new" };
    if (hash.startsWith("ws/")) {
      const [name, sub] = hash.slice(3).split("/");
      return { kind: "ws", name, mode: sub === "settings" || sub === "edit" || sub === "log" ? sub : undefined };
    }
    if (hash.startsWith("gw/")) {
      const [name, sub] = hash.slice(3).split("/");
      return { kind: "gw", name, info: sub === "info" };
    }
    const svc = services.find((s) => s.id === hash) || services[0];
    return svc ? { kind: "svc", id: svc.id } : self ? { kind: "control" } : { kind: "loading" };
  }, [hash, services, self]);

  const current = route.kind === "gw" ? gateways.find((g) => g.name === route.name) : null;
  const currentWs = route.kind === "ws" ? (wsInfo?.workspaces || []).find((w) => w.name === route.name) : null;
  const currentSvc = route.kind === "svc" ? services.find((s) => s.id === route.id) : null;
  // A service whose container we can see but isn't running gets a status
  // panel with its logs instead of an iframe full of 502s.
  const svcDown = !!(currentSvc && currentSvc.status && !isUp(currentSvc.status));

  // Iframes stay mounted once visited, so hopping between a child gateway
  // and a service doesn't reload the whole nested cascade.
  const [visited, setVisited] = useState<Record<string, boolean>>({});
  const [reloads, setReloads] = useState<Record<string, number>>({}); // frame key -> reload count
  const activeFrame =
    route.kind === "svc" && !svcDown
      ? `svc:${route.id}`
      : route.kind === "gw" && !route.info && current?.state === "running"
        ? `gw:${current.name}`
        : route.kind === "ws" && !route.mode && currentWs?.gw?.state === "running"
          ? `gw:${currentWs.gw.name}`
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

  // "Start" leaves the route pinned to the build log until the gateway
  // comes up, then drops back to the tab's natural default (the iframe).
  useEffect(() => {
    if (route.kind === "ws" && route.mode === "log" && currentWs?.gw?.state === "running") {
      location.hash = `ws/${route.name}`;
    }
  }, [route, currentWs]);

  const frames = [
    ...services.map((s) => ({ key: `svc:${s.id}`, src: s.path, title: s.name })),
    ...gateways
      .filter((g) => g.state === "running")
      .map((g) => ({ key: `gw:${g.name}`, src: g.path, title: `Gateway ${g.name}` })),
  ].filter((f) => visited[f.key]);

  const popout = activeFrame ? frames.find((f) => f.key === activeFrame)?.src ?? null : null;

  useEffect(() => {
    const label =
      route.kind === "svc"
        ? SERVICES[route.id]?.name
        : route.kind === "gw"
          ? `gw/${route.name}`
          : route.kind === "ws"
            ? route.name
            : route.kind === "ws-new"
              ? "Add workspace"
              : route.kind === "gw-new"
                ? "Launch gateway"
                : route.kind === "jobs"
                  ? "Deployment"
                  : route.kind === "control"
                    ? "Control"
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
        refreshSelf={refreshSelf}
        onWsChanged={() => {
          void refreshWorkspaces();
          void refresh();
        }}
        onReload={activeFrame ? () => setReloads((r) => ({ ...r, [activeFrame]: (r[activeFrame] || 0) + 1 })) : null}
      />
      <main title="Main canvas — shows the selected service, gateway, or panel">
        {frames.map((f) => (
          <iframe key={`${f.key}#${reloads[f.key] || 0}`} src={f.src} title={f.title} hidden={f.key !== activeFrame} />
        ))}
        {!activeFrame && (
          <div className="panel-wrap" title="Panel area — forms and status pages that aren't an embedded sub-app">
            {selfErr && <div className="banner error">Console API unreachable: {selfErr}</div>}
            {route.kind === "control" && self && (
              <ControlPage
                self={self}
                wsInfo={wsInfo}
                gateways={gateways}
                gwErr={gwErr}
                refreshSelf={refreshSelf}
                catalog={jobsCatalog}
                runs={jobRuns}
                onRan={refreshJobRuns}
              />
            )}
            {route.kind === "svc" && svcDown && currentSvc?.status && <ServiceDown svc={{ ...currentSvc, status: currentSvc.status }} />}
            {route.kind === "gw" && <GatewayDetails name={route.name} gw={current} gwErr={gwErr} />}
            {route.kind === "ws" &&
              (wsErr ? (
                <div className="panel wide">
                  <div className="banner error">Can't load workspaces: {wsErr}</div>
                </div>
              ) : !currentWs ? (
                <div className="panel wide">
                  <h2>{route.name}</h2>
                  <p className="muted">This workspace doesn't exist (any more).</p>
                </div>
              ) : route.mode === "log" ? (
                <WorkspaceStartLog ws={currentWs} />
              ) : (
                <WorkspaceDetails
                  key={route.name}
                  ws={currentWs}
                  mode={route.mode === "edit" ? "edit" : undefined}
                  dirs={wsInfo?.dirs || []}
                  profiles={self?.profiles || []}
                  onChanged={() => {
                    void refreshWorkspaces();
                    void refresh();
                  }}
                />
              ))}
            {route.kind === "ws-new" && (
              <WorkspaceAddPage
                dirs={wsInfo?.dirs || []}
                profiles={self?.profiles || []}
                onChanged={() => {
                  void refreshWorkspaces();
                  void refresh();
                }}
              />
            )}
            {route.kind === "gw-new" && (
              <AdHocLaunchPage
                own={wsInfo?.own && !wsInfo.own.pending ? wsInfo.own : null}
                self={self}
                onChanged={() => {
                  void refreshWorkspaces();
                  void refresh();
                }}
              />
            )}
            {route.kind === "jobs" && self && <DeploymentPage catalog={jobsCatalog} runs={jobRuns} onRan={refreshJobRuns} />}
            {route.kind === "loading" && !selfErr && <p className="muted">Loading…</p>}
          </div>
        )}
      </main>
    </div>
  );
}

interface SidebarProps {
  self: Self | null;
  services: ComputedService[];
  gateways: Gateway[];
  wsInfo: WorkspaceInfo | null;
  route: Route;
  collapsed: boolean;
  onToggle: () => void;
  popout: string | null;
  refresh: () => Promise<void>;
  refreshSelf: () => Promise<void>;
  onWsChanged: () => void;
  onReload: (() => void) | null;
}

// A confirm-inline (Stop or Remove) replacing one row's actions, keyed by
// its sidebar row key ("ws:<name>" or "gw:<name>").
type RowConfirm = { key: string; kind: "stop" | "remove" } | null;

function Sidebar({ self, services, gateways, wsInfo, route, collapsed, onToggle, popout, refresh, refreshSelf, onWsChanged, onReload }: SidebarProps) {
  const [confirm, setConfirm] = useState<RowConfirm>(null);
  const [purge, setPurge] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const own = health(self?.bundle);

  const localCatalog = useMemo(() => Object.fromEntries((self?.localServices || []).map((s) => [s.name, s])), [self?.localServices]);
  const localOps = self?.localOps || {};
  const [svcBusy, setSvcBusy] = useState<string | null>(null);

  async function toggleLocal(svc: string, running: boolean) {
    setErr(null);
    setSvcBusy(svc);
    try {
      await (running ? api.stopLocal(svc) : api.startLocal(svc));
    } catch (e) {
      setErr(`${svc}: ${e instanceof Error ? e.message : String(e)}`);
    }
    setSvcBusy(null);
    void refreshSelf();
  }

  // Compose services this bundle knows how to start but hasn't yet created
  // a container for at all (as opposed to one that's merely stopped).
  const startable = Object.keys(localCatalog).filter((n) => !(self?.bundle || []).some((b) => b.service === n));

  async function stop(name: string, action: Parameters<typeof api.stop>[1]) {
    setConfirm(null);
    setErr(null);
    try {
      await api.stop(name, action);
    } catch (e) {
      setErr(`${name}: ${e instanceof Error ? e.message : String(e)}`);
    }
    void refresh();
  }

  async function startWs(name: string) {
    setErr(null);
    try {
      await api.startWorkspaceGateway(name);
      // eslint-disable-next-line react-hooks/immutability -- navigating via `location.hash` after an async action completes, same as every other `location.hash =` in this migration; the compiler's heuristic only flags this one occurrence of the pattern.
      location.hash = `ws/${name}/log`;
    } catch (e) {
      setErr(`${name}: ${e instanceof Error ? e.message : String(e)}`);
    }
    onWsChanged();
  }

  async function removeWs(name: string) {
    setConfirm(null);
    setErr(null);
    try {
      await api.removeWorkspace(name, purge);
    } catch (e) {
      setErr(`${name}: ${e instanceof Error ? e.message : String(e)}`);
    }
    onWsChanged();
  }

  const managedNames = new Set((wsInfo?.workspaces || []).map((w) => w.gateway));
  const adHoc = gateways.filter((g) => !managedNames.has(g.name));
  const wsRunning = (wsInfo?.workspaces || []).filter((w) => w.gw).length + adHoc.length;

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

      <ul title="Control — this gateway's services and messages">
        <li>
          <a href="#control" className={route.kind === "control" ? "active" : ""} title="Start and stop services, and send messages">
            <span className="ico">
              <span className={`dot ${!self ? "" : !self.dockerOK || own.degraded ? "degraded" : "running"}`} />
            </span>
            <span className="label">
              Control
              <small>
                {!self ? (
                  "…"
                ) : self.requests?.length ? (
                  <span className="warn-text">
                    {self.requests.length} request{self.requests.length > 1 ? "s" : ""} to review
                  </span>
                ) : self.dockerOK ? (
                  `${self.role} · ${self.id}`
                ) : (
                  "docker unreachable"
                )}
              </small>
            </span>
          </a>
        </li>
      </ul>

      <div className="section" title="Services section — this bundle's sub-apps (deightz, dozzle, …)">
        Services <span className="count">{self ? `${own.up}/${own.total}` : "…"}</span>
        <RowMenu
          className="section-action"
          icon="+"
          title="Start a service"
          items={
            startable.length > 0
              ? startable.map((n) => ({ label: SERVICES[n]?.name || n, onClick: () => void toggleLocal(n, false) }))
              : [{ label: "Nothing to start", disabled: true }]
          }
        />
      </div>
      <ul>
        {services.map((s) => {
          const svc = localCatalog[s.compose];
          const op = localOps[s.compose];
          const opBusy = op && (op.state === "starting" || op.state === "stopping");
          const running = !!(s.status && isUp(s.status));
          return (
            <li key={s.id} className="gw-item">
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
              {svc && (
                <span className="row-actions label">
                  <RowMenu
                    title={`Actions for ${s.name}`}
                    items={[
                      {
                        label: opBusy ? op.phase || op.state : running ? "Stop" : "Start",
                        disabled: svcBusy === s.compose || !!opBusy,
                        onClick: () => void toggleLocal(s.compose, running),
                      },
                    ]}
                  />
                </span>
              )}
            </li>
          );
        })}
      </ul>

      <div
        className="section"
        title="Workspaces section — each configured and ad hoc workspace gateway, cascaded under /gw/<name>/"
      >
        Workspaces <span className="count">{wsInfo ? `${wsRunning} running` : "…"}</span>
        <a className="icon-btn section-action" href="#gw-new" title="Launch an ad hoc gateway">
          ⌁
        </a>
        <a className="icon-btn section-action" href="#ws-new" title="Add a workspace">
          +
        </a>
      </div>
      <ul>
        {(wsInfo?.workspaces || []).map((w) => {
          const gw = w.gw;
          const key = `ws:${w.name}`;
          const h = gw ? health(gw.services) : null;
          const busy = gw && (gw.state === "starting" || gw.state === "stopping");
          const dot = !gw ? "" : gw.state === "running" && h?.degraded ? "degraded" : gw.state;
          const running = gw?.state === "running";
          return (
            <li key={key} className="gw-item">
              <a
                href={`#ws/${w.name}`}
                className={route.kind === "ws" && route.name === w.name ? "active" : ""}
                title={`${w.name} (${w.source}): ${gw ? gw.phase || gw.state : "not running"}`}
              >
                <span className="ico">
                  <span className={`dot ${dot}`} />
                </span>
                <span className="label">
                  {w.name}
                  <small className={gw?.state === "error" || (gw?.state === "running" && h?.degraded) ? "error-text" : ""}>
                    {!gw
                      ? "not running"
                      : busy
                        ? gw.phase || gw.state
                        : gw.state === "error"
                          ? "failed to start"
                          : `${gw.state} · ${h?.up}/${h?.total} up · ${gw.docker}`}
                  </small>
                </span>
              </a>
              {confirm?.key === key ? (
                <div className="confirm label">
                  {confirm.kind === "stop" && gw ? (
                    <StopConfirm gw={gw} compact onStop={(action) => stop(gw.name, action)} onKeep={() => setConfirm(null)} />
                  ) : (
                    <span className="confirm-inline">
                      {w.source === "github" && (
                        <label className="check" title={`Also delete the d8zws-${w.name} volume and everything in it`}>
                          <input type="checkbox" checked={purge} onChange={(e) => setPurge(e.target.checked)} /> delete volume
                        </label>
                      )}
                      <button className="danger" onClick={() => removeWs(w.name)}>
                        Remove
                      </button>
                      <button onClick={() => setConfirm(null)}>Keep</button>
                    </span>
                  )}
                </div>
              ) : (
                <span className="row-actions label">
                  <RowMenu
                    title={`Actions for ${w.name}`}
                    items={[
                      { label: "Settings", href: `#ws/${w.name}/settings` },
                      ...(!gw ? [{ label: "Start", onClick: () => void startWs(w.name) }] : []),
                      { label: "View", href: `#ws/${w.name}`, disabled: !running },
                      { label: "Edit", href: `#ws/${w.name}/edit` },
                      ...(gw && gw.state !== "stopping"
                        ? [{ label: "Stop", onClick: () => setConfirm({ key, kind: "stop" }) }]
                        : []),
                      ...(!gw
                        ? [
                            {
                              label: "Remove",
                              danger: true,
                              onClick: () => {
                                setPurge(false);
                                setConfirm({ key, kind: "remove" });
                              },
                            },
                          ]
                        : []),
                    ]}
                  />
                </span>
              )}
            </li>
          );
        })}
        {adHoc.map((g) => {
          const h = health(g.services);
          const busy = g.state === "starting" || g.state === "stopping";
          const dot = g.state === "running" && h.degraded ? "degraded" : g.state;
          const key = `gw:${g.name}`;
          const running = g.state === "running";
          return (
            <li key={key} className="gw-item">
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
                    {busy ? g.phase || g.state : g.state === "error" ? "failed to start" : `${g.state} · ${h.up}/${h.total} up · ${g.docker}`}
                  </small>
                </span>
              </a>
              {confirm?.key === key ? (
                <div className="confirm label">
                  <StopConfirm gw={g} compact onStop={(action) => stop(g.name, action)} onKeep={() => setConfirm(null)} />
                </div>
              ) : (
                <span className="row-actions label">
                  <RowMenu
                    title={`Actions for ${g.name}`}
                    items={[
                      { label: "Settings", href: `#gw/${g.name}/info` },
                      { label: "View", href: `#gw/${g.name}`, disabled: !running },
                      ...(g.state !== "stopping" ? [{ label: "Stop", onClick: () => setConfirm({ key, kind: "stop" }) }] : []),
                    ]}
                  />
                </span>
              )}
            </li>
          );
        })}
      </ul>

      <ul className="bottom" title="Deployment — Docker Hub images and cloud gateways (deployment.yaml)">
        <li>
          <a href="#jobs" className={route.kind === "jobs" ? "active" : ""} title="Docker Hub images and cloud gateways">
            <span className="ico">⚙</span>
            <span className="label">Deployment</span>
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

function ControlPage({
  self,
  wsInfo,
  gateways,
  gwErr,
  refreshSelf,
  catalog,
  runs,
  onRan,
}: {
  self: Self;
  wsInfo: WorkspaceInfo | null;
  gateways: Gateway[];
  gwErr: string | null;
  refreshSelf: () => Promise<void>;
  catalog: Job[];
  runs: Record<string, JobRun>;
  onRan: () => void;
}) {
  const wsJobs = useMemo(() => (catalog || []).filter((j) => j.workspace), [catalog]);
  const first = ["pull", "diff_repo", "push_branch", "create_pull_request"];
  const mainJobs = first.map((n) => wsJobs.find((j) => j.name === n)).filter((j): j is Job => !!j);
  const moreJobs = wsJobs.filter((j) => !first.includes(j.name));

  const facts: Fact[] = [{ label: "Mounted at", value: <code>{self.prefix || ""}/</code> }];
  if (self.workspace) {
    facts.push({
      label: "Workspace",
      value: (
        <>
          <code>{self.workspace.name}</code> <span className="muted">({self.workspace.source})</span>
        </>
      ),
    });
  }
  facts.push({
    label: "Runs on",
    value: (
      <>
        {self.isolated ? "its own dind sidecar (isolated bundle)" : "a shared docker daemon"}{" "}
        {self.dockerOK ? <span className="muted">(docker {self.docker})</span> : <span className="error-text">unreachable: {self.dockerError}</span>}
      </>
    ),
  });

  return (
    <Panel wide>
      <EntityCard
        level="page"
        title={
          <>
            Control <code>{self.id}</code>
          </>
        }
        tag={self.role}
        facts={facts}
        error={self.bundleError || gwErr}
      >
        {self.workspace && wsInfo?.own?.pending && <WorkspacePending name={wsInfo.own.name} />}
        {self.workspace && wsInfo?.own && !wsInfo.own.pending && (
          <OwnWorkspacePanel ws={wsInfo.own} mainJobs={mainJobs} moreJobs={moreJobs} runs={runs} onRan={onRan} />
        )}

        {(self.requests || []).length > 0 && (
          <EntitySection title="Requests from children">
            {(self.requests ?? []).map((r) => (
              <RequestRow key={r.gateway} req={r} gw={gateways.find((g) => g.name === r.gateway)} onDone={refreshSelf} />
            ))}
          </EntitySection>
        )}

        <EntityCard title="Messages">
          {self.role !== "root" && <AskParent />}
          <Messenger base={self.prefix ?? ""} title="This gateway" />
          <ParentAndSiblings prefix={self.prefix ?? ""} />
          {gateways
            .filter((g) => g.state === "running")
            .map((g) => (
              <Messenger key={g.name} base={g.prefix} title={`Child: ${g.workspace ? `${g.workspace} (gw/${g.name})` : `gw/${g.name}`}`} />
            ))}
        </EntityCard>
      </EntityCard>
    </Panel>
  );
}

// A child's request, queued here until you act on it.
function RequestRow({ req, gw, onDone }: { req: Request; gw: Gateway | undefined; onDone: () => void }) {
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const act = (fn: () => Promise<unknown>) => async () => {
    setBusy(true);
    setErr(null);
    setConfirm(false);
    try {
      await fn();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
    onDone();
  };
  return (
    <EntityRow
      dot="degraded"
      id={
        <a href={`#gw/${req.gateway}/info`}>
          <code>{gw?.workspace ? `${gw.workspace} (gw/${req.gateway})` : `gw/${req.gateway}`}</code>
        </a>
      }
      tag={req.kind}
      summary={
        <>
          {req.reason || <span className="muted">no reason given</span>} <span className="muted">· {new Date(req.time).toLocaleTimeString()}</span>
        </>
      }
      summaryTone="none"
      actions={
        confirm ? (
          <span className="confirm-inline">
            <button className="danger" disabled={busy} onClick={act(() => api.approveRequest(req.gateway))}>
              Restart gw/{req.gateway} now
            </button>
            <button onClick={() => setConfirm(false)}>Not yet</button>
          </span>
        ) : (
          <>
            <button className="link-btn" disabled={busy} onClick={() => setConfirm(true)}>
              Approve
            </button>
            <button className="link-btn stop" disabled={busy} onClick={act(() => api.dismissRequest(req.gateway))}>
              Dismiss
            </button>
          </>
        )
      }
      detail={
        <>
          {confirm && (
            <p className="muted ws-stop">
              Stops its bundle (and any gateways below it) and starts it again from the same spec, on the current images.
              Workspace files are kept.
            </p>
          )}
          {err && <div className="banner error">{err}</div>}
        </>
      }
    />
  );
}

// A child can't act on its parent; it can ask. The parent's Control tab
// queues the request until you approve it there.
function AskParent() {
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  async function ask(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    setMsg(null);
    try {
      await api.askParent("restart", reason.trim());
      setMsg("Asked. Approve it on the parent's Control tab when you're ready.");
      setReason("");
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : String(e2));
    }
    setBusy(false);
  }
  return (
    <>
      <h3>Parent</h3>
      <form className="messenger-compose" onSubmit={ask}>
        <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Why (optional)…" disabled={busy} />
        <button className="primary" disabled={busy}>
          {busy ? "Asking…" : "Request restart"}
        </button>
      </form>
      {msg && <p className="muted">{msg}</p>}
      {err && <div className="banner error">{err}</div>}
    </>
  );
}

function ServiceDown({ svc }: { svc: ComputedService & { status: ServiceStatus } }) {
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

function GatewayDetails({ name, gw, gwErr }: { name: string; gw: Gateway | null | undefined; gwErr: string | null }) {
  if (!gw) {
    return (
      <div className="panel">
        <h2>gw/{name}</h2>
        <p className="muted">{gwErr ? `Can't list gateways: ${gwErr}` : "This gateway doesn't exist (any more)."}</p>
      </div>
    );
  }
  const busy = gw.state === "starting" || gw.state === "stopping";
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
        <dd>
          {gw.workspace ? (
            <a href={`#ws/${gw.workspace}/settings`}>
              <code>{gw.workspace}</code>
            </a>
          ) : (
            <span className="muted">none</span>
          )}
        </dd>
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

      <GatewayLivePanel name={name} gw={gw} />
    </div>
  );
}
