import { useState } from "react";
import { api } from "./api";
import { LogView } from "./LogView";
import type { Job, JobRun } from "./types";

/** Props for {@link JobRow}. */
export interface JobRowProps {
  job: Job;
  run: JobRun | null | undefined;
  onRan: () => void;
  /** The sidebar's project selection ("" / "self" = this repo). */
  project?: string;
}

/** One runnable job (Docker Hub / GCP / GitHub), with its args form and live state. */
export function JobRow({ job, run, onRan, project = "" }: JobRowProps) {
  const id = `${job.group}/${job.name}`;
  const [args, setArgs] = useState<Record<string, string>>({});
  const [confirm, setConfirm] = useState(false);
  const [open, setOpen] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const opBusy = run?.state === "starting";
  const argsOk = (job.args || []).every(
    (a) => (!a.required || (args[a.name] || "").trim() || a.default) && (!a.choices || a.choices.length > 0)
  );

  async function fire() {
    setConfirm(false);
    setErr(null);
    try {
      // Blank args are left out: the server fills in their defaults.
      const sent = Object.fromEntries(
        Object.entries(args)
          .map(([k, v]): [string, string] => [k, v.trim()])
          .filter(([, v]) => v)
      );
      await api.runJob(job.group, job.name, project, sent);
      setOpen(true);
      onRan();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
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
          if (a.choices) {
            return (
              <select
                key={a.name}
                value={args[a.name] || a.default || ""}
                title={a.label || a.name}
                disabled={a.choices.length === 0}
                onChange={(e) => setArgs((x) => ({ ...x, [a.name]: e.target.value }))}
              >
                {a.choices.length === 0 && <option value="">no {a.label || a.name}s in deployment.yaml</option>}
                {a.choices.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            );
          }
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
