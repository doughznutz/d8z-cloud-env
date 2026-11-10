import type { ReactNode } from "react";
import { useState } from "react";
import { EntityRow } from "./design/entity";
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
      {services.map((s) => {
        const up = isUp(s);
        const expanded = open === s.service;
        return (
          <EntityRow
            key={s.container}
            open={expanded}
            dot={s.state === "absent" ? "" : up ? (s.status.includes("unhealthy") ? "degraded" : "running") : s.state}
            id={<code>{s.service}</code>}
            summary={s.status}
            summaryTone={up ? "muted" : "error"}
            actions={
              <>
                {actions && actions(s)}
                {s.state !== "absent" && (
                  <button className="link-btn" onClick={() => setOpen(expanded ? null : s.service)}>
                    {expanded ? "Hide logs" : "Logs"}
                  </button>
                )}
              </>
            }
            expanded={expanded && <LogView load={logsFor(s.service)} live={up || s.state === "restarting"} />}
          />
        );
      })}
    </div>
  );
}
