import type { HealthSummary, ServiceStatus } from "./types";

/** Whether a service's reported state counts as "up". */
export const isUp = (s: ServiceStatus | null | undefined): boolean => s?.state === "running";

/** Summarises a bundle's containers: "3/4 up", plus whether anything is down. */
export function health(services: ServiceStatus[] | null | undefined): HealthSummary {
  const list = services || [];
  const up = list.filter(isUp).length;
  return { up, total: list.length, degraded: up < list.length };
}
