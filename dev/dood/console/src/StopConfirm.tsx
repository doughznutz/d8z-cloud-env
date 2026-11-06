import { useEffect, useState } from "react";
import { api } from "./api";
import type { Gateway, StopAction, WorkspaceStatus } from "./types";

/** Props for {@link StopConfirm}. */
export interface StopConfirmProps {
  gw: Gateway;
  onStop: (action: StopAction) => void;
  onKeep: () => void;
  compact?: boolean;
}

/**
 * Confirmation for stopping a gateway. It first asks the gateway itself
 * (over NATS, via our gw-server) what it has open, so a stop never silently
 * walks away from it:
 *   - its config changes (its branch of what we gave it): merge them into
 *     ours, save them as a branch to start a child from later (the
 *     default), or discard them - onStop(action) with merge|branch|discard;
 *   - for a workspace's gateway, uncommitted or untracked files and commits
 *     not on GitHub. Those files survive a stop (they live in the
 *     workspace's volume or host directory); what's at stake is forgetting
 *     about them, or deleting the volume afterwards.
 */
export function StopConfirm({ gw, onStop, onKeep, compact = false }: StopConfirmProps) {
  const check = gw.state === "running";
  const [st, setSt] = useState<WorkspaceStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!check) return;
    let live = true;
    api
      .workspaceStatus(gw.name)
      .then((r) => live && setSt(r))
      .catch((e: unknown) => live && setErr(e instanceof Error ? e.message : String(e)));
    return () => {
      live = false;
    };
  }, [check, gw.name]);

  const cfg = st?.config || [];
  const dirty = st && !st.clean && (st.changed || st.untracked || st.unpushed);
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
          {st && cfg.length > 0 && (
            <div>
              <span className="warn-text">
                ⚠ {cfg.length} config change{cfg.length === 1 ? "" : "s"}
              </span>
              {!compact && <pre className="log stop-files">{cfg.join("\n")}</pre>}
            </div>
          )}
          {st && !st.error && gw.workspace && !dirty && (
            <span className="ok-text">
              ✓ {st.git ? `nothing open on ${st.branch || "the checkout"}` : st.present ? "not a git checkout" : "workspace is empty"}
            </span>
          )}
          {dirty && st && !st.error && (
            <>
              <span className="warn-text">
                ⚠ {parts.join(", ")}
                {st.branch && !compact && (
                  <>
                    {" "}
                    on <code>{st.branch}</code>
                  </>
                )}
              </span>{" "}
              {review}
              {!compact && (
                <>
                  {st.files && st.files.length > 0 && <pre className="log stop-files">{st.files.join("\n")}</pre>}
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
        {cfg.length > 0 ? (
          <>
            <button className="danger" onClick={() => onStop("merge")} title="Apply its config changes to this gateway's config, then stop it">
              Merge &amp; stop
            </button>
            <button className="danger" onClick={() => onStop("branch")} title="Save its config changes as a branch here (start a child from it later), then stop it">
              Save branch &amp; stop
            </button>
            <button className="danger" onClick={() => onStop("discard")} title="Drop its config changes and stop it">
              Discard &amp; stop
            </button>
          </>
        ) : (
          <button className="danger" onClick={() => onStop("branch")} disabled={check && !st && !err}>
            {dirty ? "Stop anyway" : "Stop"}
          </button>
        )}
        <button onClick={onKeep}>Keep</button>
      </span>
    </div>
  );
}
