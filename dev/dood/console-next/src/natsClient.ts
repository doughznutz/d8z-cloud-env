/**
 * console-next's own NATS client, dialing Go's existing plain client port
 * directly (server/nats.go's NatsClientPort - unauthenticated by design on
 * internal_net, explicitly meant for exactly this kind of direct client)
 * instead of going through Go's HTTP /api/nats/* bridge.
 *
 * Reads/writes the exact same per-gateway subject and JetStream stream
 * server/nats.go itself uses (gw.<dotted-id>.inbox / gw_<id>), so this is a
 * different transport for the same data, not a new protocol - no Go change
 * needed. A "td.<dotted-gw-id>." subject prefix is reserved (parallel to
 * Go's "gw." and "ctl." prefixes, but a distinct top-level token) for if
 * console-next ever needs to originate its own cross-cutting messages;
 * nothing here uses it yet, since mirroring Go's existing inbox is all
 * Phase 4 needs.
 */

import { connect, type NatsConnection } from "@nats-io/transport-node";
import { jetstreamManager } from "@nats-io/jetstream";
import { loadConfig } from "./config.js";

let ncPromise: Promise<NatsConnection> | undefined;

async function getConnection(): Promise<NatsConnection> {
  if (!ncPromise) {
    const cfg = loadConfig();
    ncPromise = connect({ servers: cfg.natsUrl }).catch((err: unknown) => {
      ncPromise = undefined;
      throw err;
    });
  }
  return ncPromise;
}

/** gwSubject mirrors server/nats.go's gwSubject: a gateway's own "gw.<dotted-id>.<suffix>" subject. */
function gwSubject(id: string, suffix: string): string {
  return `gw.${id.replaceAll("-", ".")}.${suffix}`;
}

/** streamName mirrors server/nats.go's streamName: a gateway's own JetStream stream. */
function streamName(id: string): string {
  return `gw_${id.replaceAll("-", "_")}`;
}

/** NatsInboxMessage mirrors server/nats.go's InboxMsg wire shape, so it's a drop-in replacement for what /api/nats/inbox already returned. */
export interface NatsInboxMessage {
  seq: number;
  subject: string;
  time: string;
  data: string;
}

/** natsInbox returns up to max of gateway id's own recent messages, oldest first - the same stream server/nats.go's own Inbox() reads, just read directly instead of through Go's HTTP handler. */
export async function natsInbox(id: string, max = 50): Promise<NatsInboxMessage[]> {
  const nc = await getConnection();
  const jsm = await jetstreamManager(nc);
  const name = streamName(id);
  const info = await jsm.streams.info(name);
  const out: NatsInboxMessage[] = [];
  if (info.state.messages === 0) return out;
  const clamped = max <= 0 || max > 500 ? 50 : max;
  let start = info.state.first_seq;
  if (info.state.last_seq - start + 1 > clamped) start = info.state.last_seq - clamped + 1;
  for (let seq = start; seq <= info.state.last_seq; seq++) {
    const msg = await jsm.streams.getMessage(name, { seq }).catch(() => null);
    if (!msg) continue; // purged/deleted sequence
    out.push({ seq: msg.seq, subject: msg.subject, time: msg.time.toISOString(), data: msg.string() });
  }
  return out;
}

/** natsSend publishes text into gateway id's own inbox subject and flushes - the same plain core-NATS publish server/nats.go's own Publish() does, just issued directly instead of through Go's HTTP handler. */
export async function natsSend(id: string, text: string): Promise<void> {
  const nc = await getConnection();
  nc.publish(gwSubject(id, "inbox"), text);
  await nc.flush();
}
