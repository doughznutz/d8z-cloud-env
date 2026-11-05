import { useCallback, useEffect, useState } from "react";
import { api } from "./api.js";
import { JobRow, health } from "./App.jsx";
import { Messenger } from "./Messenger.jsx";
import { StopConfirm } from "./StopConfirm.jsx";

// The Workspaces tab. Two faces, picked by the server (info.mode):
//   manager - the root (or any gateway not serving a workspace): edits
//             workspace.yaml and starts/stops one gateway per workspace.
//   gateway - a workspace's own gateway: the one place with that workspace
//             mounted read/write, so this is where pull / diff / push / PRs run.
export function WorkspacesPage({ info, err, onChanged, catalog, runs, onRan, self }) {
  if (err) return <div className="panel wide"><div className="banner error">Can't load workspaces: {err}</div></div>;
  if (!info) return <p className="muted">Loading…</p>;
  if (info.mode === "gateway" && info.pending) return <WorkspacePending name={info.workspace?.name} />;
  if (info.mode === "gateway") return <WorkspaceRepo ws={info.workspace} catalog={catalog} runs={runs} onRan={onRan} self={self} />;
  return <WorkspaceManager info={info} profiles={self?.profiles || []} onChanged={onChanged} />;
}

function WorkspaceManager({ info, profiles, onChanged }) {
  const [editing, setEditing] = useState(null); // null | "new" | workspace name
  const list = info.workspaces || [];
  const editWs = editing && editing !== "new" ? list.find((w) => w.name === editing) : null;

  return (
    <div className="panel wide">
      <h2>Workspaces</h2>
      <p className="muted">
        A workspace is a source tree to work on: a <b>GitHub repository</b>, pulled into its own docker volume, or a{" "}
        <b>local directory</b> under <code>~/projects</code>. Each one gets its own gateway, with a Deightz editor and
        the repository tools (pull, diff, push, pull requests). Those are the only containers that mount the
        workspace, so this console never touches workspace contents itself.
      </p>
      <dl className="facts">
        <dt>Config</dt>
        <dd>
          <code>{info.file}</code>{" "}
          <span className="muted">
            {info.persisted
              ? "(holds the GitHub tokens: keep it outside every repository)"
              : "(not written yet: showing the default workspace from .env; saved on your first change)"}
          </span>
        </dd>
      </dl>
      {info.error && <div className="banner error">{info.error}</div>}

      <div className="svc-table">
        {list.length === 0 && <div className="svc-head muted">No workspaces yet.</div>}
        {list.map((w) => (
          <WorkspaceRow key={w.name} ws={w} onChanged={onChanged} onEdit={() => setEditing(w.name)} />
        ))}
      </div>
      {editing === null && (
        <div className="actions">
          <button className="primary" onClick={() => setEditing("new")}>
            + Add workspace
          </button>
        </div>
      )}
      {editing !== null && (
        <WorkspaceForm
          key={editing}
          ws={editWs}
          dirs={info.dirs || []}
          profiles={profiles}
          onDone={() => {
            setEditing(null);
            onChanged();
          }}
          onCancel={() => setEditing(null)}
        />
      )}
    </div>
  );
}

function WorkspaceRow({ ws, onChanged, onEdit }) {
  const gw = ws.gw;
  const [confirm, setConfirm] = useState(null); // "stop" | "remove"
  const [purge, setPurge] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const act = (fn) => async () => {
    setErr(null);
    setBusy(true);
    setConfirm(null);
    try {
      await fn();
    } catch (e) {
      setErr(e.message);
    }
    setBusy(false);
    onChanged();
  };

  const h = gw ? health(gw.services) : null;
  const gwBusy = gw && (gw.state === "starting" || gw.state === "stopping");
  const dot = !gw ? "" : gw.state === "running" && h.degraded ? "degraded" : gw.state;
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

  return (
    <div className="svc-row">
      <div className="svc-head">
        <span className={`dot ${dot}`} title={gw ? `gw/${gw.name}: ${gw.phase || gw.state}` : "no gateway"} />
        <code>
          <b>{ws.name}</b>
        </code>
        <span className="tag">{ws.source}</span>
        <span className="svc-status">
          {where}
          <span className={ws.tokenSet ? "muted" : "warn-text"}>
            {" "}
            · {ws.tokenSet ? `token ✓${ws.tokenEnv && !ws.token ? ` (${ws.tokenEnv})` : ""}` : "no token"}
          </span>
        </span>
        <button className="link-btn" onClick={onEdit} disabled={busy}>
          Edit
        </button>
        {confirm === "remove" ? (
          <span className="confirm-inline">
            {ws.source === "github" && (
              <label className="check" title={`Also delete the d8zws-${ws.name} volume and everything in it`}>
                <input type="checkbox" checked={purge} onChange={(e) => setPurge(e.target.checked)} /> delete volume
              </label>
            )}
            <button className="danger" onClick={act(() => api.removeWorkspace(ws.name, purge))}>
              Remove
            </button>
            <button onClick={() => setConfirm(null)}>Keep</button>
          </span>
        ) : (
          <button
            className="link-btn stop"
            disabled={busy || !!gw}
            title={gw ? "Stop its gateway first" : "Remove from workspace.yaml"}
            onClick={() => setConfirm("remove")}
          >
            Remove
          </button>
        )}
      </div>
      <div className="svc-head ws-gw">
        <span className="muted">Gateway</span>
        <code>gw/{ws.gateway}</code>
        {gw ? (
          <>
            <span className={`svc-status ${gw.state === "error" || (h && h.degraded) ? "error-text" : "muted"}`}>
              {gwBusy && <span className="spinner inline" />} {gw.phase || gw.error || `${gw.state} · ${h.up}/${h.total} up · ${gw.docker}`}
            </span>
            {gw.state === "running" && <a href={`#gw/${gw.name}`}>Open →</a>}
            <a href={`#gw/${gw.name}/info`}>Details</a>
            {confirm === "stop" ? null : (
              gw.state !== "stopping" && (
                <button className="link-btn stop" disabled={busy} onClick={() => setConfirm("stop")}>
                  Stop
                </button>
              )
            )}
          </>
        ) : (
          <>
            <span className="svc-status muted">
              not running · would run {(ws.services || []).join(", ") || "traefik only"} on{" "}
              {ws.docker === "dind" ? "its own dind" : "the host daemon"}
            </span>
            <button className="link-btn" disabled={busy} onClick={act(() => api.startWorkspaceGateway(ws.name))}>
              {busy ? "Starting…" : "Start gateway"}
            </button>
          </>
        )}
      </div>
      {gw && confirm === "stop" && (
        <div className="ws-stop">
          <StopConfirm gw={gw} onStop={act(() => api.stop(gw.name))} onKeep={() => setConfirm(null)} />
        </div>
      )}
      {gw && gw.state === "running" && <Messenger base={gw.prefix} title="Messages" className="ws-messenger" />}
      {err && <div className="banner error">{err}</div>}
    </div>
  );
}

function WorkspaceForm({ ws, dirs, profiles, onDone, onCancel }) {
  const isNew = !ws;
  const [f, setF] = useState(() => ({
    name: ws?.name || "",
    source: ws?.source || "github",
    repo: ws?.repo || "",
    branch: ws?.branch || "",
    path: ws?.path || "",
    tokenEnv: ws?.tokenEnv || "",
    gateway: ws?.gateway && ws.gateway !== ws?.name?.replace(/[^a-z0-9]/g, "") ? ws.gateway : "",
    token: "",
    clearToken: false,
    services: ws?.services || ["deightz"],
    docker: ws?.docker || "host",
  }));
  const toggleSvc = (n) =>
    setF((x) => ({ ...x, services: x.services.includes(n) ? x.services.filter((s) => s !== n) : [...x.services, n] }));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);
  const set = (k) => (e) => setF((x) => ({ ...x, [k]: e.target.type === "checkbox" ? e.target.checked : e.target.value }));

  // A local workspace named after its directory unless told otherwise.
  const pickDir = (e) => {
    const path = e.target.value;
    setF((x) => ({ ...x, path, name: isNew && (!x.name || x.name === x.path.toLowerCase()) ? path.toLowerCase() : x.name }));
  };
  const pickRepo = (e) => {
    const repo = e.target.value;
    const guess = (r) => (r.split("/")[1] || "").replace(/\.git$/, "").toLowerCase();
    setF((x) => ({ ...x, repo, name: isNew && (!x.name || x.name === guess(x.repo)) ? guess(repo) : x.name }));
  };

  async function submit(e) {
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
      onDone();
    } catch (e) {
      setErr(e.message);
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
          <span>Branch <small className="muted">(optional)</small></span>
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
            <input type="checkbox" checked={f.clearToken} onChange={set("clearToken")} /> remove the stored token
          </label>
        )}
      </label>

      <fieldset className="field">
        <legend>Gateway services</legend>
        <small className="muted">
          What the workspace's gateway runs beside its traefik (the repository manager, always on). Changes apply the
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
        <span>Gateway name <small className="muted">(optional)</small></span>
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

// A workspace gateway's own view of its workspace: the repository tools.
function WorkspaceRepo({ ws, catalog, runs, onRan, self }) {
  const jobs = (catalog || []).filter((j) => j.workspace);
  const first = ["pull", "diff_repo", "push_branch", "create_pull_request"];
  const main = first.map((n) => jobs.find((j) => j.name === n)).filter(Boolean);
  const more = jobs.filter((j) => !first.includes(j.name));
  const prefix = self?.prefix ?? "";
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
        <dd>{ws.tokenSet ? "set" : <span className="warn-text">none: public pull only (set one on the root's Workspaces tab)</span>}</dd>
      </dl>
      <p className="muted">
        Push sends the workspace as it is now to a branch - a new <code>auto-update-…</code> one unless you name one -
        respecting <code>.gitignore</code> and always leaving out every <code>.env</code> and top-level dotfile.{" "}
        {ws.source === "github"
          ? "Its changes are committed here first, so the checkout stays in step with GitHub."
          : "It's pushed from a snapshot: your local directory and its git history are never written."}
      </p>
      <h3>Messages</h3>
      <Messenger base={prefix} title="This gateway" />
      <ParentAndSiblings prefix={prefix} />
      <h3>GitHub</h3>
      <div className="svc-table">
        {main.map((j) => (
          <JobRow key={j.name} job={j} run={runs[`${j.group}/${j.name}`]} onRan={onRan} />
        ))}
      </div>
      {more.length > 0 && (
        <>
          <h3>More</h3>
          <div className="svc-table">
            {more.map((j) => (
              <JobRow key={j.name} job={j} run={runs[`${j.group}/${j.name}`]} onRan={onRan} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// A workspace gateway's view onto its own parent and running siblings: a
// gw-<id>'s own GW_PREFIX is always its parent's prefix plus exactly one
// trailing "/gw/<name>" segment, so the parent's own prefix - and this
// gateway's name within it - come straight out of this gateway's own
// prefix, no extra API needed. The root never reaches here (it's never a
// workspace gateway), so a match always exists in practice.
function ParentAndSiblings({ prefix }) {
  const m = /^(.*)\/gw\/[a-z0-9]+$/.exec(prefix || "");
  const parentPrefix = m ? m[1] : null;
  const [gws, setGws] = useState(null);
  const [err, setErr] = useState(null);

  const fetchIt = useCallback(async () => {
    if (parentPrefix === null) return;
    try {
      const r = await api.gatewaysAt(parentPrefix);
      setGws(r.gateways || []);
      setErr(null);
    } catch (e) {
      setErr(e.message);
    }
  }, [parentPrefix]);

  useEffect(() => {
    fetchIt();
    const t = setInterval(fetchIt, 5000);
    return () => clearInterval(t);
  }, [fetchIt]);

  if (parentPrefix === null) return null; // this is the root - no parent

  const siblings = (gws || []).filter((g) => g.prefix !== prefix && g.state === "running");
  return (
    <>
      <h3>Parent &amp; siblings</h3>
      {err && <div className="banner error">Can't list siblings: {err}</div>}
      <Messenger base={parentPrefix} title="Parent" />
      {siblings.map((g) => (
        <Messenger key={g.prefix} base={g.prefix} title={`Sibling: gw/${g.name}`} />
      ))}
      {gws !== null && siblings.length === 0 && <p className="muted">No other running siblings.</p>}
    </>
  );
}

// A workspace gateway that hasn't heard from its parent yet: everything
// about its workspace - repo, mount, token - arrives over NATS once the
// parent has linked to it (usually within ~15 seconds of starting).
function WorkspacePending({ name }) {
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
