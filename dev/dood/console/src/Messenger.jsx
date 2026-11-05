import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api.js";

// A compose box + recent-messages feed for one gateway's NATS inbox,
// reached at `base` (that gateway's own GW_PREFIX - see api.js). Used from
// the Control tab to talk to a running workspace's gateway, its parent,
// or its siblings: anywhere in the cascade ordinary Traefik routing
// reaches, not just this console's own gw-server.
export function Messenger({ base, title, className }) {
  const [msgs, setMsgs] = useState(null);
  const [err, setErr] = useState(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const box = useRef(null);
  const stick = useRef(true);

  const fetchIt = useCallback(async () => {
    try {
      const r = await api.inbox(base);
      setMsgs(r.messages || []);
      setErr(null);
    } catch (e) {
      setErr(e.message);
    }
  }, [base]);

  useEffect(() => {
    fetchIt();
    const t = setInterval(fetchIt, 3000);
    return () => clearInterval(t);
  }, [fetchIt]);

  useEffect(() => {
    if (box.current && stick.current) box.current.scrollTop = box.current.scrollHeight;
  }, [msgs]);

  async function send(e) {
    e.preventDefault();
    const t = text.trim();
    if (!t) return;
    setBusy(true);
    setErr(null);
    try {
      await api.sendMessage(base, t);
      setText("");
      await fetchIt();
    } catch (e) {
      setErr(e.message);
    }
    setBusy(false);
  }

  return (
    <div className={["messenger", className].filter(Boolean).join(" ")}>
      {title && <div className="messenger-title muted">{title}</div>}
      <div
        ref={box}
        className="log messenger-log"
        onScroll={(e) => {
          const el = e.currentTarget;
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
        }}
      >
        {msgs === null
          ? "Loading…"
          : msgs.length === 0
          ? "No messages yet."
          : msgs.map((m) => (
              <div key={m.seq}>
                <span className="muted">{new Date(m.time).toLocaleTimeString()}</span>{" "}
                <span className="tag">{m.subject.split(".").pop()}</span> {m.data}
              </div>
            ))}
      </div>
      {err && <div className="banner error">{err}</div>}
      <form className="messenger-compose" onSubmit={send}>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Message…" disabled={busy} />
        <button className="primary" disabled={busy || !text.trim()}>
          {busy ? "Sending…" : "Send"}
        </button>
      </form>
    </div>
  );
}

// A workspace gateway's view onto its own parent and running siblings: a
// gw-<id>'s own GW_PREFIX is always its parent's prefix plus exactly one
// trailing "/gw/<name>" segment, so the parent's own prefix - and this
// gateway's name within it - come straight out of this gateway's own
// prefix, no extra API needed. The root never reaches here (it's never a
// workspace gateway), so a match always exists in practice.
export function ParentAndSiblings({ prefix }) {
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
