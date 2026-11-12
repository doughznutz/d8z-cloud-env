import { useCallback, useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { api, ApiError } from "./api";

type AuthState =
  | { kind: "loading" }
  | { kind: "no-gate" }
  | { kind: "anonymous"; error?: string }
  | { kind: "authenticated"; username: string };

/**
 * Auth gates its children behind console-next's login - but only where
 * console-next actually fronts this gateway (today, the root; a child
 * bundle's gw-server has no /auth/* routes at all, so GET /auth/me there
 * 404s rather than 401ing, and Traefik's own basicAuth remains its sole
 * gate, unchanged). A 404 is read as "no gate here" and children render
 * straight through; only a real 401 shows the sign-in form.
 */
export function Auth({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ kind: "loading" });

  const refresh = useCallback(async () => {
    try {
      const me = await api.me();
      setState({ kind: "authenticated", username: me.username });
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setState({ kind: "no-gate" });
        return;
      }
      setState({ kind: "anonymous" });
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const onSubmit = useCallback(
    (e: FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      const form = e.currentTarget;
      const username = (form.elements.namedItem("username") as HTMLInputElement).value;
      const password = (form.elements.namedItem("password") as HTMLInputElement).value;
      api
        .login(username, password)
        .then(() => refresh())
        .catch((err: unknown) => setState({ kind: "anonymous", error: err instanceof Error ? err.message : String(err) }));
    },
    [refresh],
  );

  const onLogout = useCallback(() => {
    api
      .logout()
      .then(() => setState({ kind: "anonymous" }))
      .catch(() => setState({ kind: "anonymous" }));
  }, []);

  if (state.kind === "loading") return null;
  if (state.kind === "no-gate") return <>{children}</>;

  if (state.kind === "anonymous") {
    return (
      <div style={{ display: "flex", height: "100vh", alignItems: "center", justifyContent: "center" }}>
        <form onSubmit={onSubmit} style={{ display: "flex", flexDirection: "column", gap: 8, width: 240 }}>
          <h1 style={{ fontSize: 18, margin: "0 0 8px" }}>Sign in</h1>
          {state.error && <p style={{ color: "crimson", margin: 0 }}>{state.error}</p>}
          <input name="username" placeholder="Username" autoComplete="username" required />
          <input name="password" type="password" placeholder="Password" autoComplete="current-password" required />
          <button type="submit">Sign in</button>
        </form>
      </div>
    );
  }

  return (
    <>
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, padding: "4px 12px", fontSize: 12 }}>
        <span>{state.username}</span>
        <button onClick={onLogout} style={{ fontSize: 12 }}>
          Sign out
        </button>
      </div>
      {children}
    </>
  );
}
