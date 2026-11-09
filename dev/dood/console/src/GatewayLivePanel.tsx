import { useCallback } from "react";
import { api } from "./api";
import { ServicesTable } from "./ServicesTable";
import { LogView } from "./LogView";
import type { Gateway } from "./types";

/** Props for {@link GatewayLivePanel}. */
export interface GatewayLivePanelProps {
  name: string;
  gw: Gateway;
}

/**
 * A gateway's live half: its containers (with expandable logs) and its own
 * start/stop log. Shared by the ad hoc gateway settings page and a
 * configured workspace's settings page once it has a running (or starting
 * or stopping) gateway.
 */
export function GatewayLivePanel({ name, gw }: GatewayLivePanelProps) {
  const busy = gw.state === "starting" || gw.state === "stopping";
  const loadSpawn = useCallback(() => api.log(name), [name]);
  const logsFor = useCallback((svc: string) => () => api.childServiceLogs(name, svc), [name]);

  return (
    <>
      <h3>Containers</h3>
      <ServicesTable services={gw.services || []} logsFor={logsFor} />

      <h3>Start / stop log</h3>
      <LogView load={loadSpawn} live={!!busy} empty="Nothing logged." />
    </>
  );
}
