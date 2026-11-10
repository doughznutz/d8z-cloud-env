/**
 * Shared data shapes for the console, mirroring what the gw-server backend
 * (dev/dood/server/) sends and accepts over `api.ts`. This file is the sole
 * owner of cross-file shapes; component-local prop types stay colocated in
 * their own files.
 *
 * The Jobs and Deployment/Config domains' wire shapes are generated from
 * their own Go structs (server/jobs, server/deployment -
 * server/cmd/schemagen) into ./types.gen.ts, and re-exported below under
 * their existing console-side names - so this file stays the single place
 * importers look, and a Go field rename or addition shows up here without
 * hand-editing. Request DTOs with no clean 1:1 Go match (TargetInput: the
 * Go CloudTarget the console submits isn't quite deployment.yaml's
 * CloudTarget - see its own comment) stay hand-written.
 */
export type {
  JobSpec as Job,
  JobArg,
  DeploymentResponse as Deployment,
  DockerHubView as DockerHub,
  ImageView as DeploymentImage,
  GCPView as GCP,
  TargetView as DeploymentTargetEntry,
  CloudTarget as DeploymentTarget,
  CloudState as TargetState,
  HubRepoInfo as HubRepoStatus,
  HubInput as DockerHubInput,
  GCPInput,
  HubImage as ImageInput,
} from "./types.gen";

/** One compose service's live container state, as reported by the backend. */
export interface ServiceStatus {
  service: string;
  container: string;
  /** e.g. "running" | "restarting" | "absent" | ... */
  state: string;
  /** Human-readable status text; may call out "unhealthy". */
  status: string;
}

/** Aggregate up/down count for a set of services, from {@link health}. */
export interface HealthSummary {
  up: number;
  total: number;
  degraded: boolean;
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
export interface Request {
  gateway: string;
  /** e.g. "restart" */
  kind: string;
  reason?: string;
  time: string | number;
}

/** `api.self()`'s result: everything this gateway knows about itself. */
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
  requests?: Request[];
  localServices?: LocalServiceDescriptor[];
  localOps?: Record<string, { state: string; phase?: string; error?: string }>;
}

/** The subset of {@link Self} that Workspaces.tsx needs from its parent. */
export interface GatewaySelf {
  profiles?: Profile[];
  prefix?: string;
  isolated?: boolean;
}

/** `api.gateways()` list item: one spawned/spawning child gateway. */
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

/** `api.jobRuns()` value, keyed by `"<group>/<name>"`. */
export interface JobRun {
  /** e.g. "starting" | "error" | "running" | ... */
  state: string;
  phase?: string;
  error?: string;
}

/** The hash-derived page the console is currently showing. */
export type Route =
  | { kind: "loading" }
  | { kind: "jobs" }
  | { kind: "control" }
  /** A configured workspace's tab. `mode` undefined: the natural default
   *  (settings if no live gateway is running, the iframe if one is).
   *  "settings"/"edit" and "log" are explicit, sticky overrides set by the
   *  row's dropdown menu; "log" auto-clears once the gateway comes up. */
  | { kind: "ws"; name: string; mode?: "settings" | "edit" | "log" }
  | { kind: "ws-new" }
  | { kind: "gw"; name: string; info: boolean }
  | { kind: "gw-new" }
  | { kind: "svc"; id: string };

/**
 * A workspace as stored in workspaces.yaml / returned by `api.workspaces()`.
 * See {@link OwnWorkspace} for the richer runtime shape of "this gateway's
 * own" workspace, reported live over NATS.
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
 * (as opposed to a plain workspaces.yaml list entry) - checkout presence,
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

/** `api.workspaces()`'s result. */
export interface WorkspaceInfo {
  own?: OwnWorkspace;
  workspaces: Workspace[];
  file: string;
  persisted: boolean;
  error?: string;
  /** Candidate local directories under ~/projects, for the "new local workspace" form. */
  dirs?: string[];
}

/** `api.branches()` list item: a saved branch from a stopped workspace gateway. */
export interface Branch {
  id: string;
  from: string;
  time: string | number;
  changes: string[];
}

/** One message in a gateway's NATS inbox, shown by Messenger.tsx. */
export interface InboxMessage {
  seq: number | string;
  time: string | number;
  /** Dot-delimited; the last segment is used as a display tag. */
  subject: string;
  data: string;
}

/** `api.workspaceStatus()`'s result: a workspace gateway's open git work. */
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

/** Request body for `api.spawn`: launch an ad hoc child gateway. */
export interface SpawnRequest {
  name: string;
  profiles: string[];
  docker: "host" | "dind";
  workspace: string;
  branch: string;
}

/** Request body for `api.addWorkspace`/`api.editWorkspace`. */
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

/** Options for `api.startWorkspaceGateway`. */
export interface StartWorkspaceGatewayOptions {
  branch?: string;
}

/**
 * Request body for `api.addTarget`/`api.editTarget`. Hand-written, not
 * reused from the generated CloudTarget: the console never sets `provider`
 * (the server fills it in, and rejects anything else), so this stays its
 * own narrower shape.
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

/**
 * The repeated "nothing being edited / adding a new item / editing an
 * existing item" idiom used by inline-editable lists (Deployment.tsx's
 * images/targets, Workspaces.tsx's workspace list): `null` = nothing being
 * edited, `"new"` = the add-new-item form is open, any other string = the
 * name/id of the existing item being edited.
 */
// eslint-disable-next-line @typescript-eslint/no-redundant-type-constituents -- "new" is a named state, not an arbitrary string; keeping it documents the idiom even though `string` already subsumes it for narrowing.
export type EditingKey = null | "new" | string;
