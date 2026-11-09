import type { ReactNode } from "react";
import { useState } from "react";
import { isUp } from "./health";
import { LogView } from "./LogView";
import type { ServiceStatus } from "./types";

/** Props for {@link ServicesTable}. */
export interface ServicesTableProps {
  services: ServiceStatus[];
  logsFor: (svc: string) => () => Promise<string>;
  actions?: (s: ServiceStatus) => ReactNode | null;
}

/**
 * Lists a bundle's containers with expandable logs. `actions`, if given,
 * renders extra controls (e.g. start/stop) into a row's head.
 */
export function ServicesTable({ services, logsFor, actions }: ServicesTableProps) {
  const [open, setOpen] = useState<string | null>(null);
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
