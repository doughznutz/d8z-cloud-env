import type { ChangeEvent, FormEvent, ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { EntityCard, EntityRow, EntitySection, Panel } from "./design/entity";
import { FormShell, RemoveButton, field, useSave } from "./design/form";
import { JobRow } from "./JobRow";
import { LogView } from "./LogView";
import { api } from "./api";
import type {
  Deployment,
  DeploymentImage,
  DeploymentTarget,
  DeploymentTargetEntry,
  DockerHub,
  GCP,
  HubRepoStatus,
  EditingKey,
  Job,
  JobRun,
} from "./types";

// The Deployment tab: deployment.yaml, viewed and edited here section by
// section (like workspace.yaml on the Workspaces tab) - the Docker Hub
// account and repository, the named images published into it, the GCP
// project and key, and the cloud targets - plus the jobs acting on them and a
// NATS line to each running cloud gateway. Secrets are write-only: set,
// replace or clear, never shown.

const JOB_GROUPS: Record<string, string> = { docker: "Docker Hub jobs", cloud: "Cloud jobs" };

/** Props for {@link DeploymentPage}. */
export interface DeploymentPageProps {
  catalog: Job[];
  runs: Record<string, JobRun>;
  onRan: () => void;
}

/** The Deployment tab: deployment.yaml, viewed and edited section by section. */
export function DeploymentPage({ catalog, runs, onRan }: DeploymentPageProps) {
  const [dep, setDep] = useState<Deployment | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setDep(await api.deployment());
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, []);
  useEffect(() => {
    void load();
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, [load]);

  const groups = useMemo(() => {
    const g: Record<string, Job[]> = {};
    for (const j of catalog || []) if (!j.workspace) (g[j.group] ??= []).push(j);
    return g;
  }, [catalog]);

  const ran = () => {
    onRan();
    setTimeout(load, 1000);
  };

  if (!dep && !err) return <p className="muted">Loading…</p>;
  const editable = !!dep?.editable;

  return (
    <Panel wide>
      <EntityCard
        level="page"
        title="Deployment"
        description={
          <>
            What this gateway builds and ships (named images, one tag each in one Docker Hub repository) and where it runs
            them (cloud gateways). Dangerous jobs (a push, a VM) ask you to confirm first. GitHub work lives on{" "}
            <a href="#control">Control</a>.
          </>
        }
        facts={[
          {
            label: "Config",
            value: (
              <>
                <code>{dep?.file}</code>{" "}
                <span className="muted">
                  {dep?.fromFile
                    ? "(can hold keys: kept outside every repository)"
                    : "(not written yet: showing defaults from .env; saved on your first change)"}
                </span>
              </>
            ),
          },
        ]}
        error={err || dep?.error}
      >
        {dep?.dockerHub && (
          <DockerHubSection hub={dep.dockerHub} editable={editable} onChanged={load} onRan={ran}>
            <ImagesSection
              hub={dep.dockerHub}
              services={dep.dhServices || []}
              targets={dep.targets || []}
              editable={editable}
              onChanged={load}
              runs={runs}
              onRan={ran}
            />
            <JobsSection
              title={JOB_GROUPS.docker}
              items={groups.docker}
              runs={runs}
              onRan={ran}
              selector={{ argName: "image", label: "image", choices: ["all", ...dep.dockerHub.images.map((i) => i.name)] }}
            />
          </DockerHubSection>
        )}
        {dep?.gcp && (
          <GCPSection gcp={dep.gcp} editable={editable} onChanged={load}>
            <TargetsSection
              targets={dep.targets || []}
              images={(dep.dockerHub?.images || []).map((i) => i.name)}
              publicIP={dep.publicIP}
              editable={editable}
              onChanged={load}
              runs={runs}
              onRan={ran}
            />
            <JobsSection
              title={JOB_GROUPS.cloud}
              items={groups.cloud}
              runs={runs}
              onRan={ran}
              selector={{ argName: "target", label: "target", choices: (dep.targets || []).map((t) => t.target.name) }}
            />
          </GCPSection>
        )}
      </EntityCard>
    </Panel>
  );
}

/** A single "image"/"target" selector shared by every job in a JobsSection, instead of one per row. */
interface JobsSelector {
  argName: string;
  label: string;
  choices: string[];
}

// A group's jobs, nested inside the EntityCard they act on (Docker Hub's
// account/repo jobs inside its card, GCP's VM jobs inside its own) rather
// than listed generically: deployment.yaml only ever has these two job
// groups, each owned by exactly one of the two cards. Collapsed by
// default - the row-level macros on Images/Targets cover the common case,
// and this is where you reach for anything else (pull, list, start/stop a
// VM, ...). One selector up top stands in for the "image"/"target" input
// every job here would otherwise repeat; a job that doesn't take that arg
// (list_tags, cloud/list, ...) just ignores it.
function JobsSection({
  title,
  items,
  runs,
  onRan,
  selector,
}: {
  title: string;
  items: Job[] | undefined;
  runs: Record<string, JobRun>;
  onRan: () => void;
  selector: JobsSelector;
}) {
  const [sel, setSel] = useState(selector.choices[0] || "");
  const [open, setOpen] = useState(false);
  if (!items || items.length === 0) return null;
  const fixedArgs = { [selector.argName]: sel };
  return (
    <EntitySection
      title={title}
      collapsed={!open}
      action={
        <>
          <label className="muted">
            {selector.label}{" "}
            <select value={sel} onChange={(e) => setSel(e.target.value)} disabled={selector.choices.length === 0}>
              {selector.choices.length === 0 && <option value="">none yet</option>}
              {selector.choices.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
          <button className="link-btn" onClick={() => setOpen((o) => !o)}>
            {open ? "Hide" : "Show"}
          </button>
        </>
      }
    >
      {items.map((j) => (
        <JobRow key={`${j.group}/${j.name}`} job={j} run={runs[`${j.group}/${j.name}`]} onRan={onRan} fixedArgs={fixedArgs} />
      ))}
    </EntitySection>
  );
}

// The same "confirm, run, show busy/log" idiom as JobRow, but for a job
// whose one arg is fixed by the row it's attached to (an image's name, a
// target's name) rather than picked by the user - so an image/target row
// can act on itself directly instead of only through the generic Jobs
// list above. Its run state is still the shared one keyed by "<group>/
// <name>" (the backend allows one run of a given job at a time,
// regardless of which image/target it was for), so this and the matching
// row in the Jobs section always agree on whether it's busy.
function useJobAction(label: string, group: string, name: string, argName: string, arg: string, run: JobRun | null | undefined, onRan: () => void) {
  const id = `${group}/${name}`;
  const [confirm, setConfirm] = useState(false);
  const [open, setOpen] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const busy = run?.state === "starting";

  async function fire() {
    setConfirm(false);
    setErr(null);
    try {
      await api.runJob(group, name, "", { [argName]: arg });
      setOpen(true);
      onRan();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }

  const control = (
    <>
      {confirm ? (
        <span className="confirm-inline">
          <button className="danger" onClick={fire}>
            Really {label.toLowerCase()}
          </button>
          <button onClick={() => setConfirm(false)}>Cancel</button>
        </span>
      ) : (
        <button className="link-btn" disabled={busy} onClick={() => setConfirm(true)}>
          {busy ? run?.phase || run?.state : label}
        </button>
      )}
      {run && (
        <button className="link-btn" onClick={() => setOpen((o) => !o)}>
          {open ? "Hide log" : "Log"}
        </button>
      )}
      {err && <span className="error-text">{err}</span>}
    </>
  );
  const log = open && run ? <LogView load={() => api.jobLog(id)} live={busy} empty="Nothing logged." /> : null;
  return { control, log };
}

// ---- Docker Hub: account, repository, token ----

/** The Docker Hub repo-status check: in flight, present, or absent/errored. */
// The server's HubRepoStatus never carries an "error" field itself (a
// failed request throws, api.ts's request() helper); { error } here is
// this check's own client-side state for "the request itself failed".
type RepoCheckState = { checking: true } | HubRepoStatus | { error: string } | null;

function DockerHubSection({
  hub,
  editable,
  onChanged,
  onRan,
  children,
}: {
  hub: DockerHub;
  editable: boolean;
  onChanged: () => void;
  onRan: () => void;
  children?: ReactNode;
}) {
  const [editing, setEditing] = useState(false);
  const [repo, setRepo] = useState<RepoCheckState>(null);
  const [creating, setCreating] = useState(false);
  const [vis, setVis] = useState<"public" | "private">("public");
  const [createErr, setCreateErr] = useState<string | null>(null);
  const [showLog, setShowLog] = useState(false);

  const check = useCallback(async () => {
    setRepo({ checking: true });
    try {
      setRepo(await api.hubRepoStatus());
    } catch (e) {
      setRepo({ error: e instanceof Error ? e.message : String(e) });
    }
  }, []);
  const key = `${hub.username}/${hub.repository}`;
  useEffect(() => {
    if (hub.username && hub.repository) void check();
  }, [key, check]); // eslint-disable-line react-hooks/exhaustive-deps -- `key` already encodes the identity (username/repository) this effect reacts to; adding `hub` would re-fire on every parent re-render instead of only when the repo identity changes.

  async function create() {
    setCreating(false);
    setCreateErr(null);
    try {
      await api.runJob("docker", "create_repo", "", { visibility: vis });
      setShowLog(true);
      onRan();
      setTimeout(check, 3000);
    } catch (e) {
      setCreateErr(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <EntityCard
      title="Docker Hub"
      actions={
        editable &&
        !editing && (
          <button className="link-btn" onClick={() => setEditing(true)}>
            Edit
          </button>
        )
      }
      facts={[
        {
          label: "Repository",
          value: (
            <>
              <a href={`https://hub.docker.com/r/${key}/tags`} target="_blank" rel="noreferrer">
                <code>{key}</code>
              </a>{" "}
              {repo && "checking" in repo && <span className="muted">checking…</span>}
              {repo && "error" in repo && <span className="error-text">{repo.error}</span>}
              {repo && "exists" in repo && repo.exists === true && (
                <span className="muted">
                  exists · {repo.private ? "private" : "public"} · {repo.pulls} pulls
                </span>
              )}
              {repo && "exists" in repo && repo.exists === false && <span className="warn-text">doesn't exist on Docker Hub yet</span>}{" "}
              <button className="link-btn" onClick={check}>
                Check
              </button>
              {editable && repo && "exists" in repo && repo.exists === false && !creating && (
                <>
                  {" · "}
                  <button className="link-btn" onClick={() => setCreating(true)}>
                    Create repository…
                  </button>
                </>
              )}
              {creating && (
                <span className="confirm-inline">
                  <select value={vis} onChange={(e) => setVis(e.target.value as "public" | "private")}>
                    <option value="public">public</option>
                    <option value="private">private</option>
                  </select>
                  <button className="danger" onClick={create}>
                    Create {key}
                  </button>
                  <button onClick={() => setCreating(false)}>Cancel</button>
                </span>
              )}
            </>
          ),
        },
        {
          label: "Token",
          value: hub.tokenSet ? (
            <span>
              set <span className="muted">({hub.tokenStored ? "stored in deployment.yaml" : `from .env ${hub.tokenEnv}`})</span>
            </span>
          ) : (
            <span className="error-text">missing: pushes and repository changes will fail</span>
          ),
        },
      ]}
    >
      {createErr && <div className="banner error">{createErr}</div>}
      {showLog && <LogView load={() => api.jobLog("docker/create_repo")} live empty="Starting…" />}
      {editing && (
        <DockerHubForm
          hub={hub}
          onCancel={() => setEditing(false)}
          onDone={() => {
            setEditing(false);
            onChanged();
          }}
        />
      )}
      {children}
    </EntityCard>
  );
}

function DockerHubForm({ hub, onCancel, onDone }: { hub: DockerHub; onCancel: () => void; onDone: () => void }) {
  const [f, setF] = useState({
    username: hub.username || "",
    repository: hub.repository || "",
    tokenEnv: hub.tokenEnv || "",
    token: "",
    clearToken: false,
  });
  const { busy, err, run } = useSave(onDone);
  return (
    <FormShell
      title="Edit Docker Hub"
      busy={busy}
      err={err}
      onCancel={onCancel}
      onSubmit={() => run(() => api.editDockerHub(f))}
      canSubmit={!!(f.username.trim() && f.repository.trim())}
    >
      <label className="field">
        <span>Account (namespace)</span>
        <input value={f.username} onChange={field(setF, "username")} placeholder="e.g. doughznutz" />
      </label>
      <label className="field">
        <span>Repository</span>
        <input value={f.repository} onChange={field(setF, "repository")} placeholder="e.g. doughznutz" />
        <small className="muted">
          Every image below is a tag of this one repository:{" "}
          <code>
            {f.username || "account"}/{f.repository || "repository"}:&lt;image&gt;
          </code>
          . Changing it doesn't move existing tags; use <b>Check</b> / <b>Create repository</b> after saving.
        </small>
      </label>
      <label className="field">
        <span>Access token</span>
        <input
          type="password"
          value={f.token}
          onChange={field(setF, "token")}
          autoComplete="new-password"
          placeholder={hub.tokenStored ? "•••••• stored - leave blank to keep" : "dckr_pat_… (Docker Hub → Account settings → Personal access tokens)"}
        />
        <small className="muted">Stored in deployment.yaml, never sent back to this page. Or read it from the .env instead:</small>
        <input value={f.tokenEnv} onChange={field(setF, "tokenEnv")} placeholder="token env var, e.g. DOCKER_HUB_TOKEN" />
        {hub.tokenStored && (
          <label className="check">
            <input type="checkbox" checked={f.clearToken} onChange={field(setF, "clearToken")} /> remove the stored token
          </label>
        )}
      </label>
    </FormShell>
  );
}

// ---- Images: tag -> compose build service ----

function ImagesSection({
  hub,
  services,
  targets,
  editable,
  onChanged,
  runs,
  onRan,
}: {
  hub: DockerHub;
  services: string[];
  targets: DeploymentTargetEntry[];
  editable: boolean;
  onChanged: () => void;
  runs: Record<string, JobRun>;
  onRan: () => void;
}) {
  const [editing, setEditing] = useState<EditingKey>(null);
  const im = editing && editing !== "new" ? hub.images.find((i) => i.name === editing) : null;
  const usedBy = (name: string) => targets.filter((t) => t.target.image === name).map((t) => t.target.name);
  return (
    <>
      <EntitySection
        title="Images"
        action={
          editable &&
          editing === null && (
            <button className="link-btn" onClick={() => setEditing("new")}>
              + Add image
            </button>
          )
        }
      >
        {(hub.images || []).map((i) => (
          <ImageRow
            key={i.name}
            image={i}
            usedBy={usedBy(i.name)}
            editable={editable}
            onChanged={onChanged}
            onEdit={() => setEditing(i.name)}
            run={runs["docker/build_and_push"]}
            onRan={onRan}
          />
        ))}
      </EntitySection>
      {editing !== null && (
        <ImageForm
          key={editing}
          im={im ?? null}
          services={services}
          onCancel={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            onChanged();
          }}
        />
      )}
    </>
  );
}

function ImageRow({
  image,
  usedBy,
  editable,
  onChanged,
  onEdit,
  run,
  onRan,
}: {
  image: DeploymentImage;
  usedBy: string[];
  editable: boolean;
  onChanged: () => void;
  onEdit: () => void;
  run: JobRun | null | undefined;
  onRan: () => void;
}) {
  const build = useJobAction("Build & push", "docker", "build_and_push", "image", image.name, run, onRan);
  return (
    <EntityRow
      id={
        <code>
          <b>{image.name}</b>
        </code>
      }
      summary={
        <>
          <code>{image.ref}</code> ← built by compose service <code>{image.service}</code>
          {usedBy.length > 0 && <> · runs on {usedBy.join(", ")}</>}
        </>
      }
      actions={
        <>
          {build.control}
          {editable && (
            <>
              <button className="link-btn" onClick={onEdit}>
                Edit
              </button>
              <RemoveButton fn={() => api.removeImage(image.name)} onDone={onChanged} />
            </>
          )}
        </>
      }
      expanded={build.log}
    />
  );
}

function ImageForm({
  im,
  services,
  onCancel,
  onDone,
}: {
  im: DeploymentImage | null;
  services: string[];
  onCancel: () => void;
  onDone: () => void;
}) {
  const [f, setF] = useState({ name: im?.name || "", service: im?.service || services[0] || "" });
  const { busy, err, run } = useSave(onDone);
  const opts = f.service && !services.includes(f.service) ? [...services, f.service] : services;
  return (
    <FormShell
      title={im ? `Edit image ${im.name}` : "Add image"}
      busy={busy}
      err={err}
      onCancel={onCancel}
      onSubmit={() => run(() => (im ? api.editImage(im.name, f) : api.addImage(f)))}
      canSubmit={!!(f.name.trim() && f.service)}
      submitLabel={im ? "Save" : "Add image"}
    >
      <label className="field">
        <span>Name (its tag)</span>
        <input value={f.name} onChange={field(setF, "name")} placeholder="e.g. d8z" maxLength={128} />
        <small className="muted">
          Published as <code>&lt;account&gt;/&lt;repository&gt;:{f.name || "name"}</code>.
          {im && " Renaming re-points the targets that run it; tags already pushed stay on Docker Hub."}
        </small>
      </label>
      <label className="field">
        <span>Built by</span>
        <select value={f.service} onChange={field(setF, "service")}>
          {opts.length === 0 && <option value="">no services in profile dh</option>}
          {opts.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <small className="muted">
          A <code>docker-compose.yaml</code> service in profile <code>dh</code> (by convention <code>&lt;service&gt;-dh</code>). To
          publish a new container, add its <code>-dh</code> service there first.
        </small>
      </label>
    </FormShell>
  );
}

// ---- GCP: project, zone, service-account key ----

function GCPSection({ gcp, editable, onChanged, children }: { gcp: GCP; editable: boolean; onChanged: () => void; children?: ReactNode }) {
  const [editing, setEditing] = useState(false);
  return (
    <EntityCard
      title="Google Cloud"
      actions={
        editable &&
        !editing && (
          <button className="link-btn" onClick={() => setEditing(true)}>
            Edit
          </button>
        )
      }
      facts={[
        { label: "Project", value: gcp.project ? <code>{gcp.project}</code> : <span className="error-text">not set</span> },
        { label: "Zone", value: gcp.zone ? <code>{gcp.zone}</code> : <span className="error-text">not set</span> },
        {
          label: "Service account",
          value: gcp.keyEmail ? (
            <>
              <code>{gcp.keyEmail}</code> <span className="muted">(key {gcp.credentials})</span>
            </>
          ) : (
            <span className="error-text">no key - add a service-account key (Compute Admin) with Edit</span>
          ),
        },
      ]}
    >
      {editing && (
        <GCPForm
          gcp={gcp}
          onCancel={() => setEditing(false)}
          onDone={() => {
            setEditing(false);
            onChanged();
          }}
        />
      )}
      {children}
    </EntityCard>
  );
}

function GCPForm({ gcp, onCancel, onDone }: { gcp: GCP; onCancel: () => void; onDone: () => void }) {
  const [f, setF] = useState({
    project: gcp.project || "",
    zone: gcp.zone || "",
    credentials: gcp.credentials || "gcp-sa.json",
    keyJSON: "",
    clearKey: false,
  });
  const { busy, err, run } = useSave(onDone);
  const pickFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    setF((x) => ({ ...x, keyJSON: text }));
  };
  let keyInfo: { ok: string } | { bad: string } | null = null;
  if (f.keyJSON.trim()) {
    try {
      const k = JSON.parse(f.keyJSON) as { type?: string; client_email?: string; project_id?: string };
      keyInfo = k.type === "service_account" ? { ok: `${k.client_email} (project ${k.project_id})` } : { bad: "not a service-account key" };
    } catch {
      keyInfo = { bad: "not valid JSON" };
    }
  }
  return (
    <FormShell
      title="Edit Google Cloud"
      busy={busy}
      err={err}
      onCancel={onCancel}
      onSubmit={() => run(() => api.editGCP(f))}
      canSubmit={!(keyInfo && "bad" in keyInfo)}
    >
      <label className="field">
        <span>Project ID</span>
        <input value={f.project} onChange={field(setF, "project")} placeholder="e.g. d8z-cloud-123456" />
        {keyInfo && "ok" in keyInfo && !f.project && (
          <small>
            <button
              type="button"
              className="link-btn"
              onClick={() => setF((x) => ({ ...x, project: (JSON.parse(x.keyJSON) as { project_id?: string }).project_id || "" }))}
            >
              use the key's project
            </button>
          </small>
        )}
      </label>
      <label className="field">
        <span>Zone</span>
        <input value={f.zone} onChange={field(setF, "zone")} placeholder="e.g. us-west1-b" />
        <small className="muted">Where the VMs run, e.g. us-west1-b, us-central1-a, europe-west4-a.</small>
      </label>
      <label className="field">
        <span>Service-account key</span>
        <small className="muted">
          IAM → Service accounts → (one with <b>Compute Admin</b>) → Keys → Add key → JSON. Pick the downloaded file or
          paste it. Stored in the config dir beside deployment.yaml (mode 600), never sent back to this page.
          {gcp.keyEmail && (
            <>
              {" "}
              Current: <code>{gcp.keyEmail}</code> - leave blank to keep.
            </>
          )}
        </small>
        <input type="file" accept=".json,application/json" onChange={pickFile} />
        <textarea
          className="key-input"
          value={f.keyJSON}
          onChange={field(setF, "keyJSON")}
          rows={4}
          placeholder='{"type": "service_account", ...}'
          spellCheck={false}
        />
        {keyInfo && "ok" in keyInfo && <small className="ok-text">✓ {keyInfo.ok}</small>}
        {keyInfo && "bad" in keyInfo && <small className="error-text">{keyInfo.bad}</small>}
        {gcp.keyEmail && (
          <label className="check">
            <input type="checkbox" checked={f.clearKey} onChange={field(setF, "clearKey")} /> delete the stored key
          </label>
        )}
      </label>
      <label className="field">
        <span>Key file name</span>
        <input value={f.credentials} onChange={field(setF, "credentials")} placeholder="gcp-sa.json" />
      </label>
    </FormShell>
  );
}

// ---- Cloud targets ----

function TargetsSection({
  targets,
  images,
  publicIP,
  editable,
  onChanged,
  runs,
  onRan,
}: {
  targets: DeploymentTargetEntry[];
  images: string[];
  publicIP?: string;
  editable: boolean;
  onChanged: () => void;
  runs: Record<string, JobRun>;
  onRan: () => void;
}) {
  const [editing, setEditing] = useState<EditingKey>(null);
  const t = editing && editing !== "new" ? targets.find((x) => x.target.name === editing)?.target : null;
  return (
    <>
      <EntitySection
        title="Cloud gateways"
        action={
          editable &&
          editing === null && (
            <button className="link-btn" onClick={() => setEditing("new")}>
              + Add target
            </button>
          )
        }
        empty="No targets yet. A target is a VM running one profile from your Docker Hub images."
      >
        {targets.map((x) => (
          <TargetRow
            key={x.target.name}
            t={x}
            editable={editable}
            onEdit={() => setEditing(x.target.name)}
            onChanged={onChanged}
            run={runs["cloud/deploy"]}
            onRan={onRan}
          />
        ))}
      </EntitySection>
      {editing !== null && (
        <TargetForm
          key={editing}
          t={t ?? null}
          images={images}
          publicIP={publicIP}
          onCancel={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            onChanged();
          }}
        />
      )}
    </>
  );
}

function TargetRow({
  t,
  editable,
  onEdit,
  onChanged,
  run,
  onRan,
}: {
  t: DeploymentTargetEntry;
  editable: boolean;
  onEdit: () => void;
  onChanged: () => void;
  run: JobRun | null | undefined;
  onRan: () => void;
}) {
  const { target, state } = t;
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const running = state?.status === "RUNNING";
  const dot = !state ? null : running ? (t.leafConnected ? "running" : "degraded") : state.status === "TERMINATED" ? "exited" : "starting";
  const deploy = useJobAction("Deploy", "cloud", "deploy", "target", target.name, run, onRan);

  async function send(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    setBusy(true);
    setErr(null);
    setRes(null);
    try {
      const r = await api.messageTarget(target.name, text);
      setRes(r.reply);
      setText("");
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : String(e2));
    }
    setBusy(false);
  }

  return (
    <EntityRow
      dot={dot}
      id={
        <code>
          <b>{target.name}</b>
        </code>
      }
      tag={target.profile}
      summary={
        <>
          <code>{t.ref}</code> · {target.machineType}, {target.diskGB} GB · from {(target.allowFrom || []).join(", ") || "nobody"}
        </>
      }
      actions={
        <>
          {deploy.control}
          {t.url && (
            <a href={t.url} target="_blank" rel="noreferrer">
              Open ↗
            </a>
          )}
          {editable && (
            <>
              <button className="link-btn" onClick={onEdit}>
                Edit
              </button>
              <RemoveButton fn={() => api.removeTarget(target.name)} onDone={onChanged} />
            </>
          )}
        </>
      }
      detail={
        <>
          <div className="svc-head ws-gw muted">
            {!state
              ? "not deployed - click Deploy to create its VM"
              : `VM ${state.instance}: ${state.status}${state.address ? ` · ${state.address}` : ""}${
                  running ? (t.leafConnected ? " · NATS linked" : " · NATS not linked") : ""
                }`}
          </div>
          {running && (
            <form className="svc-head ws-gw" onSubmit={send}>
              <span className="muted">NATS</span>
              <input
                value={text}
                onChange={(e) => setText(e.target.value)}
                placeholder={`message to ${target.name}'s inbox`}
                size={40}
                disabled={busy}
              />
              <button className="link-btn" disabled={busy || !text.trim()}>
                {busy ? "sending…" : "Send"}
              </button>
              {res && (
                <span>
                  <span className="ok-text">delivered</span> · reply <code>{res}</code>
                </span>
              )}
            </form>
          )}
          {err && <div className="banner error">{err}</div>}
          {(target.allowFrom || []).length === 0 && (
            <div className="banner error">No allow_from: deploy will refuse until you list who may reach it (Edit).</div>
          )}
        </>
      }
      expanded={deploy.log}
    />
  );
}

function TargetForm({
  t,
  images,
  publicIP,
  onCancel,
  onDone,
}: {
  t: DeploymentTarget | null;
  images: string[];
  publicIP?: string;
  onCancel: () => void;
  onDone: () => void;
}) {
  const [f, setF] = useState({
    name: t?.name || "",
    profile: t?.profile || "root",
    image: t?.image || (images.includes("dood") ? "dood" : images[0] || ""),
    machineType: t?.machineType || "e2-small",
    diskGB: String(t?.diskGB || 20),
    allowFrom: (t?.allowFrom || (publicIP ? [`${publicIP}/32`] : [])).join("\n"),
    forwardEnv: (t?.forwardEnv || []).join(", "),
  });
  const { busy, err, run } = useSave(onDone);
  const lines = (s: string, sep: string | RegExp) =>
    s
      .split(sep)
      .map((x) => x.trim())
      .filter(Boolean);
  const body = () => ({
    name: f.name.trim(),
    profile: f.profile,
    image: f.image,
    machineType: f.machineType.trim(),
    diskGB: parseInt(f.diskGB, 10) || 0,
    allowFrom: lines(f.allowFrom, /\s+/),
    forwardEnv: lines(f.forwardEnv, /[\s,]+/),
  });
  const mine = publicIP ? `${publicIP}/32` : null;
  return (
    <FormShell
      title={t ? `Edit target ${t.name}` : "Add cloud target"}
      busy={busy}
      err={err}
      onCancel={onCancel}
      onSubmit={() => run(() => (t ? api.editTarget(t.name, body()) : api.addTarget(body())))}
      canSubmit={!!(f.name.trim() && f.image)}
      submitLabel={t ? "Save" : "Add target"}
    >
      <label className="field">
        <span>Name</span>
        <input value={f.name} onChange={field(setF, "name")} placeholder="e.g. cloud1" maxLength={16} />
        <small className="muted">
          Lowercase letters and digits, starting with a letter. It's the gateway's NATS id and names its VM (
          <code>d8z-{f.name || "name"}</code>).
        </small>
      </label>
      <fieldset className="field">
        <legend>Profile</legend>
        <label className="check">
          <input type="radio" name="profile" checked={f.profile === "root"} onChange={() => setF((x) => ({ ...x, profile: "root" }))} />
          <span>
            <b>root</b>: dood only (console + control plane)
          </span>
        </label>
        <label className="check">
          <input
            type="radio"
            name="profile"
            checked={f.profile === "workspace"}
            onChange={() => setF((x) => ({ ...x, profile: "workspace" }))}
          />
          <span>
            <b>workspace</b>: dood + deightz
          </span>
        </label>
      </fieldset>
      <label className="field">
        <span>Gateway image</span>
        <select value={f.image} onChange={field(setF, "image")}>
          {images.map((i) => (
            <option key={i} value={i}>
              {i}
            </option>
          ))}
        </select>
        <small className="muted">The image the VM pulls and starts its profile from (normally dood).</small>
      </label>
      <label className="field">
        <span>Machine type</span>
        <input value={f.machineType} onChange={field(setF, "machineType")} placeholder="e2-small" />
        <small className="muted">e2-small (2 GB) is enough for root; workspace with Deightz wants e2-medium or larger.</small>
      </label>
      <label className="field">
        <span>Disk (GB)</span>
        <input value={f.diskGB} onChange={field(setF, "diskGB")} inputMode="numeric" />
      </label>
      <label className="field">
        <span>Allow from</span>
        <textarea rows={2} value={f.allowFrom} onChange={field(setF, "allowFrom")} placeholder="203.0.113.7/32" spellCheck={false} />
        <small className="muted">
          CIDRs (one per line) that may reach the console (:80) and the NATS leaf port (:7422). This gateway must be one
          of them to link over NATS.
          {mine && !f.allowFrom.includes(mine) && (
            <>
              {" "}
              <button
                type="button"
                className="link-btn"
                onClick={() => setF((x) => ({ ...x, allowFrom: [x.allowFrom.trim(), mine].filter(Boolean).join("\n") }))}
              >
                add this gateway's IP ({mine})
              </button>
            </>
          )}
        </small>
      </label>
      <label className="field">
        <span>
          Forward from .env <small className="muted">(optional)</small>
        </span>
        <input value={f.forwardEnv} onChange={field(setF, "forwardEnv")} placeholder="e.g. GEMINI_API_KEY, OPENAI_API_KEY" />
        <small className="muted">
          Extra .env keys the VM gets. They travel in instance metadata, readable by anyone with Compute viewer access to
          the project - forward only what that gateway needs.
        </small>
      </label>
    </FormShell>
  );
}
