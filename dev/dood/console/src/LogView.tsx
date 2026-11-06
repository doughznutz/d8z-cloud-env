import { useCallback, useEffect, useRef, useState } from "react";

/** Props for {@link LogView}. */
export interface LogViewProps {
  /** Fetches the current log text. */
  load: () => Promise<string>;
  /** Whether to keep polling `load` while mounted (default: true). */
  live?: boolean;
  /** Text shown when the log is empty. */
  empty?: string;
}

/**
 * Fetches text and keeps it fresh while `live`; sticks to the bottom unless
 * the user has scrolled up to read.
 */
export function LogView({ load, live = true, empty = "No output." }: LogViewProps) {
  const [text, setText] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const pre = useRef<HTMLPreElement | null>(null);
  const stick = useRef(true);

  const fetchIt = useCallback(async () => {
    try {
      setText(await load());
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, [load]);

  useEffect(() => {
    void fetchIt();
    if (!live) return;
    const t = setInterval(fetchIt, 2500);
    return () => clearInterval(t);
  }, [fetchIt, live]);

  useEffect(() => {
    if (pre.current && stick.current) pre.current.scrollTop = pre.current.scrollHeight;
  }, [text]);

  if (err) return <div className="banner error">{err}</div>;
  if (text === null) return <p className="muted">Loading…</p>;
  return (
    <pre
      ref={pre}
      className="log"
      onScroll={(e) => {
        const el = e.currentTarget;
        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
      }}
    >
      {text || empty}
    </pre>
  );
}
