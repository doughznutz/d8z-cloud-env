import type { ChangeEvent, Dispatch, ReactNode, SetStateAction } from "react";
import { useState } from "react";

/**
 * The shared idiom behind every inline add/edit form on the console
 * (Deployment.tsx's images/targets/accounts, Workspaces.tsx's workspace
 * list): a controlled form, a busy/error pair around its save call, and a
 * Cancel/Save footer.
 */

/** Props for {@link FormShell}. */
export interface FormShellProps {
  title: string;
  busy: boolean;
  err: string | null;
  onCancel: () => void;
  onSubmit: () => void;
  submitLabel?: string;
  canSubmit?: boolean;
  children?: ReactNode;
}

/** The shared chrome for every inline add/edit form: title, fields, error banner, Cancel/Save. */
export function FormShell({ title, busy, err, onCancel, onSubmit, submitLabel = "Save", canSubmit = true, children }: FormShellProps) {
  return (
    <form
      className="ws-form"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit();
      }}
    >
      <h3>{title}</h3>
      {children}
      {err && <div className="banner error">{err}</div>}
      <div className="actions gap">
        <button type="button" className="link-btn" onClick={onCancel}>
          Cancel
        </button>
        <button className="primary" disabled={busy || !canSubmit}>
          {busy ? "Saving…" : submitLabel}
        </button>
      </div>
    </form>
  );
}

/** Tracks busy/error around a save call, then calls onDone. */
export function useSave(onDone: () => void) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
  };
  return { busy, err, run };
}

/** A remove action that confirms inline before calling fn. */
export function RemoveButton({ fn, onDone, label = "Remove" }: { fn: () => Promise<unknown>; onDone: () => void; label?: string }) {
  const [confirm, setConfirm] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  if (!confirm)
    return (
      <>
        <button className="link-btn stop" onClick={() => setConfirm(true)}>
          {label}
        </button>
        {err && <span className="error-text">{err}</span>}
      </>
    );
  return (
    <span className="confirm-inline">
      <button
        className="danger"
        onClick={async () => {
          setConfirm(false);
          setErr(null);
          try {
            await fn();
            onDone();
          } catch (e) {
            setErr(e instanceof Error ? e.message : String(e));
          }
        }}
      >
        Really {label.toLowerCase()}
      </button>
      <button onClick={() => setConfirm(false)}>Cancel</button>
    </span>
  );
}

type FieldEvent = ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>;

/** A controlled-field onChange handler: updates key k of state via setF, handling checkboxes. */
export const field =
  <T,>(setF: Dispatch<SetStateAction<T>>, k: keyof T) =>
  (e: FieldEvent) => {
    const target = e.target;
    const v = target instanceof HTMLInputElement && target.type === "checkbox" ? target.checked : target.value;
    setF((x) => ({ ...x, [k]: v }));
  };
