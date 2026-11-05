import { useEffect, useState } from "react";
import { api } from "./api.js";

// Confirmation for stopping a gateway. For a workspace's gateway it first
// asks the gateway itself (over NATS, via our gw-server) whether there's
// open work - uncommitted or untracked files, commits not on GitHub - so a
// stop never silently walks away from it. The files themselves survive a
// stop (they live in the workspace's volume or host directory); what's at
// stake is forgetting about them, or deleting the volume afterwards.
export function StopConfirm({ gw, onStop, onKeep, compact = false }) {
  const check = !!gw.workspace && gw.state === "running";
  const [st, setSt] = useState(null);
  const [err, setErr] = useState(null);

  useEffect(() => {
    if (!check) return;
    let live = true;
    api
      .workspaceStatus(gw.name)
      .then((r) => live && setSt(r))
      .catch((e) => live && setErr(e.message));
    return () => {
      live = false;
    };
  }, [check, gw.name]);

  const dirty = st && !st.clean;
  const parts = st
    ? [
        st.changed && `${st.changed} uncommitted`,
        st.untracked && `${st.untracked} untracked`,
        st.unpushed && `${st.unpushed} unpushed commit${st.unpushed === 1 ? "" : "s"}`,
      ].filter(Boolean)
    : [];
  const review = (
    <a href={`${gw.path}#workspaces`} target="_blank" rel="noopener" title="Open the workspace's repository tools to push first">
      {compact ? "push first ↗" : "Open its Workspace tab to push first ↗"}
    </a>
  );

  return (
    <div className={`stop-confirm ${compact ? "compact" : ""}`}>
      {check && (
        <div className="stop-check">
          {!st && !err && (
            <span className="muted">
              <span className="spinner inline" /> checking for open changes…
            </span>
          )}
          {err && <span className="warn-text">couldn't check for open changes: {err}</span>}
          {st && st.error && <span className="warn-text">{st.error}</span>}
          {st && !st.error && !dirty && (
            <span className="ok-text">
              ✓ {st.git ? `nothing open on ${st.branch || "the checkout"}` : st.present ? "not a git checkout" : "workspace is empty"}
            </span>
          )}
          {dirty && !st.error && (
            <>
              <span className="warn-text">
                ⚠ {parts.join(", ")}
                {st.branch && !compact && <> on <code>{st.branch}</code></>}
              </span>{" "}
              {review}
              {!compact && (
                <>
                  {st.files?.length > 0 && <pre className="log stop-files">{st.files.join("\n")}</pre>}
                  <div className="muted">
                    Stopping keeps these files in the workspace{st.source === "github" ? "'s volume" : " directory"} -
                    they're only gone if you then remove the workspace and delete its volume.
                  </div>
                </>
              )}
            </>
          )}
        </div>
      )}
      <span className="confirm-inline">
        <button className="danger" onClick={onStop} disabled={check && !st && !err}>
          {dirty ? "Stop anyway" : "Stop"}
        </button>
        <button onClick={onKeep}>Keep</button>
      </span>
    </div>
  );
}
