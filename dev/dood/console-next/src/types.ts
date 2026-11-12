/**
 * Shared data shapes for console-next's goclient (src/goclient.ts), mirroring
 * console/src/types.ts but trimmed to what a server-side client actually
 * needs - no UI-only shapes (Route, EditingKey, HealthSummary, ...).
 *
 * The Jobs and Deployment domains' wire shapes are generated from their own
 * Go structs (server/jobs, server/deployment - server/cmd/schemagen) into
 * ./types.gen.ts, the same file console/src/types.gen.ts is, and re-exported
 * below under their console-side names, so a Go field rename shows up here
 * without hand-editing.
 */
export type {
  JobSpec as Job,
  DeploymentResponse as Deployment,
  HubRepoInfo as HubRepoStatus,
  HubInput as DockerHubInput,
  GCPInput,
  HubImage as ImageInput,
} from "./types.gen.js";

/** One compose service's live container state, as reported by the backend. */
export interface ServiceStatus {
  service: string;
  container: string;
  /** e.g. "running" | "restarting" | "absent" | ... */
  state: string;
  /** Human-readable status text; may call out "unhealthy". */
  status: string;
}

/** A local (non-gateway) service the backend can start/stop directly. */
export interface LocalServiceDescriptor {
  name: string;
}

/** A named selectable profile (job arg choice, spawn profile, ...). */
export interface Profile {
  name: string;
  label: string;
}

/** A child→parent request queued on the parent's Control tab. */
export interface ParentRequest {
  gateway: string;
  /** e.g. "restart" */
  kind: string;
  reason?: string;
  time: string | number;
}

/** `goclient.self()`'s result: everything this gateway knows about itself. */
export interface Self {
  id: string;
  role: string;
  /** Mount prefix: "" for the root gateway, or e.g. "/gw/xyz" for a child. */
  prefix?: string;
  dockerOK: boolean;
  docker: string;
  dockerError?: string;
  /** Whether this gateway runs its own (dind) docker daemon vs. a shared one. */
  isolated: boolean;
  /** Selectable service profiles a workspace or ad hoc gateway can run. */
  profiles?: Profile[];
  bundleError?: string;
  /** This bundle's own containers. */
  bundle?: ServiceStatus[];
  /** Ids into the sidebar's SERVICES catalog. */
  services?: string[];
  dozzlePath?: string;
  workspace?: { name: string; source: string };
  requests?: ParentRequest[];
  localServices?: LocalServiceDescriptor[];
  localOps?: Record<string, { state: string; phase?: string; error?: string }>;
}

/** `goclient.gateways()` list item: one spawned/spawning child gateway. */
export interface Gateway {
  name: string;
  /** e.g. "running" | "starting" | "stopping" | "error" | ... */
  state: string;
  phase?: string;
  error?: string;
  /** Workspace name this ad hoc gateway is working on, if any. */
  workspace?: string;
  project: string;
  srcProject?: string;
  prefix: string;
  /** "dind" | "host" */
  docker: string;
  profiles?: string[];
  upstream?: string;
  /** iframe src once running. */
  path?: string;
  services?: ServiceStatus[];
}

/** `goclient.jobRuns()` value, keyed by `"<group>/<name>"`. */
export interface JobRun {
  /** e.g. "starting" | "error" | "running" | ... */
  state: string;
  phase?: string;
  error?: string;
}

/**
 * A workspace as stored in workspace.yaml / returned by `goclient.workspaces()`.
 */
export interface Workspace {
  name: string;
  source: "github" | "local";
  repo?: string;
  branch?: string;
  path?: string;
  tokenEnv?: string;
  tokenSet: boolean;
  /** Gateway name managing this workspace. */
  gateway: string;
  services?: string[];
  docker: "host" | "dind";
  /** The live gateway object, if one is currently running for it. */
  gw?: Gateway;
}

/**
 * The extra runtime fields reported for a gateway's own assigned workspace
 * (as opposed to a plain workspace.yaml list entry) - checkout presence,
 * git state, and whether it's still being provisioned.
 */
export interface OwnWorkspace extends Workspace {
  pending?: boolean;
  /** Mount path inside this gateway. */
  dir?: string;
  writable?: boolean;
  present?: boolean;
  git?: boolean;
  head?: string;
}

/** `goclient.workspaces()`'s result. */
export interface WorkspaceInfo {
  own?: OwnWorkspace;
  workspaces: Workspace[];
  file: string;
  persisted: boolean;
  error?: string;
  /** Candidate local directories under ~/projects, for the "new local workspace" form. */
  dirs?: string[];
}

/** `goclient.branches()` list item: a saved branch from a stopped workspace gateway. */
export interface Branch {
  id: string;
  from: string;
  time: string | number;
  changes: string[];
}

/** `goclient.workspaceStatus()`'s result: a workspace gateway's open git work. */
export interface WorkspaceStatus {
  config?: string[];
  clean?: boolean;
  changed?: number;
  untracked?: number;
  unpushed?: number;
  error?: string;
  git?: boolean;
  present?: boolean;
  branch?: string;
  files?: string[];
  source?: string;
}

/** What to do with a workspace gateway's uncommitted/unpushed work on stop. */
export type StopAction = "merge" | "branch" | "discard";

/** Request body for `goclient.spawn`: launch an ad hoc child gateway. */
export interface SpawnRequest {
  name: string;
  profiles: string[];
  docker: "host" | "dind";
  workspace: string;
  branch: string;
}

/** Request body for `goclient.addWorkspace`/`goclient.editWorkspace`. */
export interface WorkspaceInput {
  name: string;
  source: "github" | "local";
  repo: string;
  branch: string;
  /** Only set when source === "local". */
  path: string;
  tokenEnv: string;
  gateway: string;
  token: string;
  clearToken: boolean;
  services: string[];
  docker: "host" | "dind";
}

/** Options for `goclient.startWorkspaceGateway`. */
export interface StartWorkspaceGatewayOptions {
  branch?: string;
}

/**
 * Request body for `goclient.addTarget`/`goclient.editTarget`. Hand-written,
 * not reused from the generated CloudTarget: the caller never sets
 * `provider` (the server fills it in, and rejects anything else), so this
 * stays its own narrower shape.
 */
export interface TargetInput {
  name: string;
  profile: string;
  image: string;
  machineType: string;
  diskGB: number;
  allowFrom: string[];
  forwardEnv: string[];
}
