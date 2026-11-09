import type { ChangeEvent, FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import { health } from "./health";
import { LogView } from "./LogView";
import { StopConfirm } from "./StopConfirm";
import { GatewayLivePanel } from "./GatewayLivePanel";
import type { Branch, GatewaySelf, OwnWorkspace, Profile, Workspace } from "./types";

/**
 * Workspaces are rendered as sidebar rows (see App.tsx's `Sidebar`), one per
 * configured workspace plus one per ad hoc gateway. This file holds the
 * canvas pages those rows' tabs and dropdown menus open:
 *   - {@link WorkspaceDetails}: a configured workspace's settings page
 *     (facts, Edit, Start/Stop/Remove), plus its gateway's live info once
 *     one is running.
 *   - {@link WorkspaceStartLog}: the transient "starting…" log page shown
 *     right after Start, until the gateway comes up.
 *   - {@link WorkspaceAddPage} / {@link AdHocLaunchPage}: the "+ Add
 *     workspace" / "Launch ad hoc gateway" forms, opened from the icons
 *     next to the sidebar's Workspaces heading.
 *   - {@link OwnWorkspacePanel} / {@link WorkspacePending}: a workspace
 *     gateway's own view of the workspace its parent assigned it (rendered
 *     on the Control tab, not here - these aren't something you start/stop).
 */

// A workspace as stored in workspace.yaml, with the new-workspace defaults
// in its empty state.
function emptyForm(ws: Workspace | null) {
  return {
    name: ws?.name || "",
    source: ws?.source || "github",
    repo: ws?.repo || "",
    branch: ws?.branch || "",
    path: ws?.path || "",
    tokenEnv: ws?.tokenEnv || "",
    gateway: ws?.gateway && ws.gateway !== ws.name.replace(/[^a-z0-9]/g, "") ? ws.gateway : "",
    token: "",
    clearToken: false,
    services: ws?.services || ["deightz"],
    docker: ws?.docker || "host",
  };
}

/** Props for {@link WorkspaceForm}. */
export interface WorkspaceFormProps {
  ws: Workspace | null;
  dirs: string[];
  profiles: Profile[];
  onDone: (name: string) => void;
  onCancel: () => void;
}

/** The add/edit form for a workspace.yaml entry. */
export function WorkspaceForm({ ws, dirs, profiles, onDone, onCancel }: WorkspaceFormProps) {
  const isNew = !ws;
  const [f, setF] = useState(() => emptyForm(ws));
  const toggleSvc = (n: string) =>
    setF((x) => ({ ...x, services: x.services.includes(n) ? x.services.filter((s) => s !== n) : [...x.services, n] }));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const set = (k: "name" | "branch" | "token" | "tokenEnv" | "gateway") => (e: ChangeEvent<HTMLInputElement>) =>
    setF((x) => ({ ...x, [k]: e.target.value }));
  const setClearToken = (e: ChangeEvent<HTMLInputElement>) => setF((x) => ({ ...x, clearToken: e.target.checked }));

  // A local workspace named after its directory unless told otherwise.
  const pickDir = (e: ChangeEvent<HTMLSelectElement>) => {
    const path = e.target.value;
    setF((x) => ({ ...x, path, name: isNew && (!x.name || x.name === x.path.toLowerCase()) ? path.toLowerCase() : x.name }));
  };
  const pickRepo = (e: ChangeEvent<HTMLInputElement>) => {
    const repo = e.target.value;
    const guess = (r: string) => (r.split("/")[1] || "").replace(/\.git$/, "").toLowerCase();
    setF((x) => ({ ...x, repo, name: isNew && (!x.name || x.name === guess(x.repo)) ? guess(repo) : x.name }));
  };

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    const body = {
      name: f.name.trim(),
      source: f.source,
      repo: f.repo.trim(),
      branch: f.branch.trim(),
      path: f.source === "local" ? f.path : "",
      tokenEnv: f.tokenEnv.trim(),
      gateway: f.gateway.trim(),
      token: f.token.trim(),
      clearToken: f.clearToken,
      services: f.services,
      docker: f.docker,
    };
    try {
      await (isNew ? api.addWorkspace(body) : api.editWorkspace(ws.name, body));
      onDone(body.name);
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : String(e2));
    }
    setBusy(false);
  }

  return (
    <form className="ws-form" onSubmit={submit}>
      <h3>{isNew ? "Add workspace" : `Edit ${ws.name}`}</h3>
      <fieldset className="field">
        <legend>Source</legend>
        <label className="check">
          <input type="radio" name="source" checked={f.source === "github"} onChange={() => setF((x) => ({ ...x, source: "github" }))} />
          <span>
            <b>GitHub repository</b>: pulled into its own docker volume (<code>d8zws-&lt;name&gt;</code>) by the
            workspace's gateway.
          </span>
        </label>
        <label className="check">
          <input type="radio" name="source" checked={f.source === "local"} onChange={() => setF((x) => ({ ...x, source: "local" }))} />
          <span>
            <b>Local directory</b>: one already under <code>~/projects</code> on the host, bind-mounted as is.
          </span>
        </label>
      </fieldset>

      {f.source === "local" && (
        <label className="field">
          <span>Directory</span>
          <select value={f.path} onChange={pickDir}>
            <option value="" disabled>
              — pick a directory in ~/projects —
            </option>
            {dirs.map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>
        </label>
      )}

      <label className="field">
        <span>Repository {f.source === "local" && <small className="muted">(optional: where diff / push / PRs go)</small>}</span>
        <input value={f.repo} onChange={pickRepo} placeholder="owner/name or https://github.com/owner/name" />
      </label>

      <label className="field">
        <span>Name</span>
        <input value={f.name} onChange={set("name")} disabled={!isNew} placeholder="e.g. my-repo" maxLength={64} />
        <small className="muted">
          {isNew ? "Lowercase letters, digits, . _ -. Also names its volume and gateway." : "Can't be renamed (it names the volume)."}
        </small>
      </label>

      {f.source === "github" && (
        <label className="field">
          <span>
            Branch <small className="muted">(optional)</small>
          </span>
          <input value={f.branch} onChange={set("branch")} placeholder="blank: the repository's default branch" />
        </label>
      )}

      <label className="field">
        <span>GitHub token</span>
        <input
          type="password"
          value={f.token}
          onChange={set("token")}
          autoComplete="new-password"
          placeholder={ws?.tokenSet ? "•••••• set - leave blank to keep" : "ghp_… / github_pat_… (blank: public pull only)"}
        />
        <small className="muted">
          Stored in workspace.yaml, never sent back to this page. Only this workspace's gateway gets it. Or read it from
          the .env instead:
        </small>
        <input value={f.tokenEnv} onChange={set("tokenEnv")} placeholder="token env var, e.g. GITHUB_TOKEN" />
        {ws?.tokenSet && (
          <label className="check">
            <input type="checkbox" checked={f.clearToken} onChange={setClearToken} /> remove the stored token
          </label>
        )}
      </label>

      <fieldset className="field">
        <legend>Gateway services</legend>
        <small className="muted">
          What the workspace's gateway runs beside its dood (the repository manager, always on). Changes apply the
          next time the gateway starts.
        </small>
        {profiles
          .filter((p) => p.name !== "dozzle")
          .map((p) => (
            <label key={p.name} className="check">
              <input type="checkbox" checked={f.services.includes(p.name)} onChange={() => toggleSvc(p.name)} />
              {p.label} <code>{p.name}</code>
            </label>
          ))}
      </fieldset>

      <fieldset className="field">
        <legend>Where the gateway runs</legend>
        <label className="check">
          <input type="radio" name="docker" checked={f.docker === "host"} onChange={() => setF((x) => ({ ...x, docker: "host" }))} />
          <span>
            <b>host</b>: on this gateway's daemon. Starts fast and reuses images, but its docker-using services can see
            the other workspaces' containers. It shares this daemon's Dozzle.
          </span>
        </label>
        <label className="check">
          <input type="radio" name="docker" checked={f.docker === "dind"} onChange={() => setF((x) => ({ ...x, docker: "dind" }))} />
          <span>
            <b>dind</b>: inside its own dind sidecar, which mounts only this workspace. Fully isolated, with its own
            Dozzle; images are copied in first, so the first start is slow.
          </span>
        </label>
      </fieldset>

      <label className="field">
        <span>
          Gateway name <small className="muted">(optional)</small>
        </span>
        <input value={f.gateway} onChange={set("gateway")} placeholder="blank: derived from the name" maxLength={16} />
      </label>

      {err && <div className="banner error">{err}</div>}
      <div className="actions gap">
        <button type="button" className="link-btn" onClick={onCancel}>
          Cancel
        </button>
        <button className="primary" disabled={busy || !f.name.trim() || (f.source === "local" && !f.path)}>
          {busy ? "Saving…" : isNew ? "Add workspace" : "Save"}
        </button>
      </div>
    </form>
  );
}

/** Props for {@link WorkspaceDetails}. */
export interface WorkspaceDetailsProps {
  ws: Workspace;
  /** "edit" opens straight into the edit form (set by the sidebar's Edit menu item). */
  mode?: "edit";
  dirs: string[];
  profiles: Profile[];
  onChanged: () => void;
}

/**
 * A configured workspace's settings page: facts, Edit, Start/Stop/Remove,
 * plus (once it has a live gateway) that gateway's containers and log.
 */
export function WorkspaceDetails({ ws, mode, dirs, profiles, onChanged }: WorkspaceDetailsProps) {
  const [editing, setEditing] = useState(mode === "edit");
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [purge, setPurge] = useState(false);
  const [confirmStop, setConfirmStop] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const gw = ws.gw;

  const act = (fn: () => Promise<unknown>) => async () => {
    setErr(null);
    setBusy(true);
    setConfirmRemove(false);
    setConfirmStop(false);
    try {
      await fn();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
    onChanged();
  };

  async function start() {
    setErr(null);
    setBusy(true);
    try {
      await api.startWorkspaceGateway(ws.name);
      location.hash = `ws/${ws.name}/log`;
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
    onChanged();
  }

  if (editing) {
    return (
      <div className="panel wide">
        <WorkspaceForm
          ws={ws}
          dirs={dirs}
          profiles={profiles}
          onDone={() => {
            setEditing(false);
            onChanged();
          }}
          onCancel={() => setEditing(false)}
        />
      </div>
    );
  }

  const where =
    ws.source === "github" ? (
      <>
        <code>{ws.repo}</code>
        {ws.branch && <span className="muted"> @ {ws.branch}</span>}
      </>
    ) : (
      <>
        <code>~/projects/{ws.path}</code>
        {ws.repo && <span className="muted"> → {ws.repo}</span>}
      </>
    );

  const h = gw ? health(gw.services) : null;
  const gwBusy = gw && (gw.state === "starting" || gw.state === "stopping");
  const dot = !gw ? "" : gw.state === "running" && h?.degraded ? "degraded" : gw.state;

  return (
    <div className="panel wide">
      <h2>
        <span className={`dot ${dot}`} /> {ws.name} <span className="tag">{ws.source}</span>
        {gw?.state === "running" && (
          <a className="h-link" href={`#ws/${ws.name}`}>
            Open console →
          </a>
        )}
      </h2>
      <dl className="facts">
        <dt>Location</dt>
        <dd>{where}</dd>
        <dt>Token</dt>
        <dd>
          <span className={ws.tokenSet ? "muted" : "warn-text"}>
            {ws.tokenSet ? `set${ws.tokenEnv ? ` (${ws.tokenEnv})` : ""}` : "none: public pull only"}
          </span>
        </dd>
        <dt>Gateway</dt>
        <dd>
          <code>gw/{ws.gateway}</code>{" "}
          {gw ? (
            <span className={`svc-status ${gw.state === "error" || (h && h.degraded) ? "error-text" : "muted"}`}>
              {gwBusy && <span className="spinner inline" />} {gw.phase || gw.error || `${gw.state} · ${h?.up}/${h?.total} up · ${gw.docker}`}
            </span>
          ) : (
            <span className="muted">
              not running · would run {(ws.services || []).join(", ") || "dood only"} on{" "}
              {ws.docker === "dind" ? "its own dind" : "the host daemon"}
            </span>
          )}
        </dd>
      </dl>

      {err && <div className="banner error">{err}</div>}

      <div className="actions gap">
        <button className="link-btn" onClick={() => setEditing(true)} disabled={busy}>
          Edit
        </button>
        {!gw && (
          <button className="primary" onClick={start} disabled={busy}>
            {busy ? "Starting…" : "Start gateway"}
          </button>
        )}
        {gw && gw.state !== "stopping" && !confirmStop && (
          <button className="link-btn stop" onClick={() => setConfirmStop(true)} disabled={busy}>
            Stop
          </button>
        )}
        {!gw &&
          (confirmRemove ? (
            <span className="confirm-inline">
              {ws.source === "github" && (
                <label className="check" title={`Also delete the d8zws-${ws.name} volume and everything in it`}>
                  <input type="checkbox" checked={purge} onChange={(e) => setPurge(e.target.checked)} /> delete volume
                </label>
              )}
              <button className="danger" onClick={act(() => api.removeWorkspace(ws.name, purge))}>
                Remove
              </button>
              <button onClick={() => setConfirmRemove(false)}>Keep</button>
            </span>
          ) : (
            <button className="link-btn stop" disabled={busy} onClick={() => setConfirmRemove(true)}>
              Remove
            </button>
          ))}
      </div>

      {gw && confirmStop && (
        <div className="ws-stop">
          <StopConfirm gw={gw} onStop={(action) => act(() => api.stop(gw.name, action))()} onKeep={() => setConfirmStop(false)} />
        </div>
      )}

      {gw && <GatewayLivePanel name={ws.gateway} gw={gw} />}
    </div>
  );
}

/** The transient "starting…" page shown right after Start, until the gateway comes up (App.tsx then flips to its iframe). */
export function WorkspaceStartLog({ ws }: { ws: Workspace }) {
  const gw = ws.gw;
  const loadSpawn = useCallback(() => api.log(ws.gateway), [ws.gateway]);
  return (
    <div className="panel wide">
      <h2>
        <span className={`dot ${gw?.state || "starting"}`} /> {ws.name} <small className="muted">{gw?.state || "starting"}…</small>
      </h2>
      {gw?.phase && (
        <p className="phase">
          <span className="spinner" /> {gw.phase}
        </p>
      )}
      {gw?.error && <div className="banner error">{gw.error}</div>}
      <LogView load={loadSpawn} live empty="Nothing logged yet." />
    </div>
  );
}

/** The "+ Add workspace" page, opened from the sidebar's Workspaces heading. */
export function WorkspaceAddPage({ dirs, profiles, onChanged }: { dirs: string[]; profiles: Profile[]; onChanged: () => void }) {
  return (
    <div className="panel wide">
      <h2>Add workspace</h2>
      <p className="muted">
        A workspace is a source tree to work on: a <b>GitHub repository</b>, pulled into its own docker volume, or a{" "}
        <b>local directory</b> under <code>~/projects</code>. Each one gets its own gateway, with a Deightz editor and
        the repository tools (pull, diff, push, pull requests). Those are the only containers that mount the
        workspace, so this console never touches workspace contents itself.
      </p>
      <WorkspaceForm
        ws={null}
        dirs={dirs}
        profiles={profiles}
        onDone={(name) => {
          onChanged();
          location.hash = `ws/${name}`;
        }}
        onCancel={() => {
          location.hash = "control";
        }}
      />
    </div>
  );
}

interface LaunchForm {
  name: string;
  services: string[];
  docker: "host" | "dind";
  withWs: boolean;
  branch: string;
}

/**
 * The "launch ad hoc gateway" page: whatever you want to try, nested under
 * this gateway (gw/<this>/gw/<name>/), not in workspace.yaml and gone when
 * stopped. By default it runs on this gateway's own docker and, if this
 * gateway serves a workspace, works on it.
 */
export function AdHocLaunchPage({ own, self, onChanged }: { own: OwnWorkspace | null; self: GatewaySelf | null; onChanged: () => void }) {
  const [f, setF] = useState<LaunchForm>({ name: "", services: ["deightz"], docker: "host", withWs: true, branch: "" });
  const useWs = !!own && f.withWs;
  const [branches, setBranches] = useState<Branch[]>([]);
  const loadBranches = useCallback(() => {
    api
      .branches()
      .then((r) => setBranches(r.branches || []))
      .catch(() => {});
  }, []);
  useEffect(() => {
    loadBranches();
    const t = setInterval(loadBranches, 10000);
    return () => clearInterval(t);
  }, [loadBranches]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const toggleSvc = (n: string) =>
    setF((x) => ({ ...x, services: x.services.includes(n) ? x.services.filter((s) => s !== n) : [...x.services, n] }));

  async function launch(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    const name = f.name.trim();
    try {
      await api.spawn({
        name,
        profiles: f.services,
        docker: f.docker,
        workspace: useWs && own ? own.name : "",
        branch: f.branch,
      });
      onChanged();
      location.hash = `gw/${name}`;
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : String(e2));
    }
    setBusy(false);
  }

  return (
    <div className="panel wide">
      <h2>Launch ad hoc gateway</h2>
      <p className="muted">
        An ad hoc gateway isn't in workspace.yaml - whatever you want to try, nested under this gateway and gone once
        stopped, unless you save its config changes as a branch first.
      </p>
      {branches.length > 0 && (
        <>
          <h3>Saved config branches</h3>
          <p className="muted">
            Config changes of children stopped without merging. Launch a child from one to pick up where it left off.
          </p>
          <BranchesList branches={branches} onLaunch={(id) => setF((x) => ({ ...x, branch: id }))} onDeleted={loadBranches} />
        </>
      )}
      <form className="ws-form" onSubmit={launch}>
        <label className="field">
          <span>Name</span>
          <input value={f.name} onChange={(e) => setF((x) => ({ ...x, name: e.target.value }))} placeholder="e.g. review" maxLength={16} />
          <small className="muted">
            Lowercase letters and digits. Mounted at{" "}
            <code>
              {self?.prefix}/gw/{f.name || "<name>"}/
            </code>
            .
          </small>
        </label>
        {own && (
          <label className="check">
            <input type="checkbox" checked={f.withWs} onChange={(e) => setF((x) => ({ ...x, withWs: e.target.checked }))} /> work on
            this gateway's workspace (<code>{own.name}</code>)
          </label>
        )}
        <fieldset className="field">
          <legend>Services</legend>
          {(self?.profiles || [])
            .filter((p) => p.name !== "dozzle")
            .map((p) => (
              <label key={p.name} className="check">
                <input type="checkbox" checked={f.services.includes(p.name)} onChange={() => toggleSvc(p.name)} />
                {p.label} <code>{p.name}</code>
              </label>
            ))}
        </fieldset>
        {branches.length > 0 && (
          <label className="field">
            <span>Config</span>
            <select value={f.branch} onChange={(e) => setF((x) => ({ ...x, branch: e.target.value }))}>
              <option value="">this gateway's config</option>
              {branches.map((b) => (
                <option key={b.id} value={b.id}>
                  this gateway's config + branch {b.id}
                </option>
              ))}
            </select>
          </label>
        )}
        <fieldset className="field">
          <legend>Docker</legend>
          <label className="check">
            <input type="radio" checked={f.docker === "host"} onChange={() => setF((x) => ({ ...x, docker: "host" }))} />
            <span>
              <b>this gateway's docker</b> ({self?.isolated ? "its dind" : "shared daemon"})
            </span>
          </label>
          <label className="check">
            <input type="radio" checked={f.docker === "dind"} onChange={() => setF((x) => ({ ...x, docker: "dind" }))} />
            <span>
              <b>isolated</b>: its own dind sidecar
            </span>
          </label>
        </fieldset>
        {err && <div className="banner error">{err}</div>}
        <div className="actions gap">
          <button type="button" className="link-btn" onClick={() => (location.hash = "control")}>
            Cancel
          </button>
          <button className="primary" disabled={busy || !/^[a-z][a-z0-9]{0,15}$/.test(f.name.trim())}>
            {busy ? "Launching…" : "Launch"}
          </button>
        </div>
      </form>
    </div>
  );
}

function BranchesList({ branches, onLaunch, onDeleted }: { branches: Branch[]; onLaunch: (id: string) => void; onDeleted: () => void }) {
  return (
    <div className="svc-table">
      {branches.map((b) => (
        <BranchRow key={b.id} b={b} onLaunch={() => onLaunch(b.id)} onDeleted={onDeleted} />
      ))}
    </div>
  );
}

function BranchRow({ b, onLaunch, onDeleted }: { b: Branch; onLaunch: () => void; onDeleted: () => void }) {
  const [open, setOpen] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  async function del() {
    setErr(null);
    try {
      await api.deleteBranch(b.id);
      onDeleted();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }
  return (
    <div className="svc-row">
      <div className="svc-head">
        <code>{b.id}</code>
        <span className="svc-status muted">
          from {b.from} · {new Date(b.time).toLocaleString()} · {(b.changes || []).length} change
          {(b.changes || []).length === 1 ? "" : "s"}
        </span>
        <button className="link-btn" onClick={() => setOpen((o) => !o)}>
          {open ? "Hide" : "Changes"}
        </button>
        <button className="link-btn" onClick={onLaunch}>
          Launch from it
        </button>
        {confirm ? (
          <span className="confirm-inline">
            <button className="danger" onClick={del}>
              Delete
            </button>
            <button onClick={() => setConfirm(false)}>Keep</button>
          </span>
        ) : (
          <button className="link-btn stop" onClick={() => setConfirm(true)}>
            Delete
          </button>
        )}
      </div>
      {open && <pre className="log stop-files">{(b.changes || []).join("\n")}</pre>}
      {err && <div className="banner error">{err}</div>}
    </div>
  );
}

/** A workspace gateway's own view of its workspace: the repository tools. Rendered on the Control tab. */
export function OwnWorkspacePanel({ ws }: { ws: OwnWorkspace }) {
  return (
    <div className="panel wide">
      <h2>
        Workspace <code>{ws.name}</code> <small className="muted">{ws.source}</small>
      </h2>
      <dl className="facts">
        <dt>Repository</dt>
        <dd>{ws.repo ? <code>{ws.repo}</code> : <span className="muted">none (local only)</span>}</dd>
        <dt>Mounted at</dt>
        <dd>
          <code>{ws.dir}</code>{" "}
          <span className={ws.writable ? "muted" : "error-text"}>{ws.writable ? "(read/write)" : "(read-only here)"}</span>
        </dd>
        <dt>Checkout</dt>
        <dd>
          {!ws.present ? (
            <span className="warn-text">empty: Pull to clone {ws.repo || "it"}</span>
          ) : ws.git ? (
            <>
              git, on <code>{ws.head}</code>
            </>
          ) : (
            <span className="muted">files, not a git checkout (diff / push compare against the repo's default branch)</span>
          )}
        </dd>
        <dt>Token</dt>
        <dd>{ws.tokenSet ? "set" : <span className="warn-text">none: public pull only (set one on the parent's Workspaces tab)</span>}</dd>
      </dl>
      <p className="muted">
        Push sends the workspace as it is now to a branch - a new <code>auto-update-…</code> one unless you name one -
        respecting <code>.gitignore</code> and always leaving out every <code>.env</code> and top-level dotfile.{" "}
        {ws.source === "github"
          ? "Its changes are committed here first, so the checkout stays in step with GitHub."
          : "It's pushed from a snapshot: your local directory and its git history are never written."}
      </p>
      <p className="muted">Its GitHub jobs (pull, diff, push, pull requests) are below, on this same Control tab.</p>
    </div>
  );
}

/**
 * A workspace gateway that hasn't heard from its parent yet: everything
 * about its workspace - repo, mount, token - arrives over NATS once the
 * parent has linked to it (usually within ~15 seconds of starting).
 */
export function WorkspacePending({ name }: { name: string }) {
  return (
    <div className="panel wide">
      <h2>
        Workspace <code>{name}</code> <small className="muted">awaiting assignment</small>
      </h2>
      <p className="phase">
        <span className="spinner" /> Waiting for the parent gateway to send this workspace's details over NATS…
      </p>
      <p className="muted">
        This gateway was started for <code>{name}</code> but is told its repository, where it's mounted and its GitHub
        token only by its parent, sealed with a key that only the two of them hold. The parent links up within its next
        reconcile pass (every 15 seconds); a fresh workspace then pulls itself.
      </p>
    </div>
  );
}
