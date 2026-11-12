/**
 * Generic reverse proxy for everything console-next doesn't wrap with a
 * typed route in src/apiRoutes.ts: console/src/api.ts's inbox/sendMessage/
 * gatewaysAt (addressed by an arbitrary `base` prefix into another gateway
 * in the cascade, not a fixed path), the iframe sub-apps (deightz/og/
 * dozzle), and nested child-gateway mounts (/gw/<name>/...) that Traefik -
 * not gw-server - fans out by path. Forwards the incoming request to the
 * configured Go server/Traefik entrypoint byte-for-byte.
 */

import type { Context } from "hono";
import { loadConfig } from "./config.js";

const cfg = loadConfig();

// Hop-by-hop headers, or ones fetch()/undici recompute on their own - must
// not be forwarded verbatim between hops.
const stripRequestHeaders = new Set(["host", "connection", "content-length"]);
const stripResponseHeaders = new Set(["content-encoding", "content-length", "connection", "transfer-encoding"]);

/** proxyPass forwards the current request to the Go server/Traefik entrypoint this console-next instance fronts, and relays the response verbatim. */
export async function proxyPass(c: Context): Promise<Response> {
  const incoming = c.req.raw;
  const incomingUrl = new URL(incoming.url);
  const upstreamUrl = new URL(incomingUrl.pathname + incomingUrl.search, cfg.upstream);

  const headers = new Headers(incoming.headers);
  for (const h of stripRequestHeaders) headers.delete(h);
  if (cfg.authHeader && !headers.has("authorization")) headers.set("authorization", cfg.authHeader);

  const hasBody = incoming.method !== "GET" && incoming.method !== "HEAD";
  const res = await fetch(upstreamUrl, {
    method: incoming.method,
    headers,
    body: hasBody ? await incoming.arrayBuffer() : undefined,
    redirect: "manual",
  });

  const resHeaders = new Headers(res.headers);
  for (const h of stripResponseHeaders) resHeaders.delete(h);
  return new Response(res.body, { status: res.status, headers: resHeaders });
}
