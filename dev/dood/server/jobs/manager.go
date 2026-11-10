package jobs

import (
	"context"
	"io"
	"time"
)

// Manager is what the jobs package needs from gw-server's Manager that
// isn't already a plain, directly-importable value (a deployment.Store, an
// optracker.Tracker - see NewCatalog) - kept narrow so Workspaces, Gateways
// and NATS stay exactly as they are.
type Manager interface {
	// WorkspaceName is this gateway's assigned workspace name ("" on a
	// manager gateway).
	WorkspaceName() string
	// ProjectsRoot is ~/projects on the host ("" if this gateway has none
	// mounted).
	ProjectsRoot() string
	// GatewayID is this gateway's own GW_ID.
	GatewayID() string

	// Assignment is this workspace gateway's assignment from its parent, or
	// nil (not a workspace gateway, or not assigned yet).
	Assignment() *Assignment

	// NatsAvailable reports whether the embedded NATS server is running.
	NatsAvailable() bool
	// NatsPing pings gateway id over NATS, returning its reply.
	NatsPing(id, text string, timeout time.Duration) (string, error)
	// Reconcile dials any newly-reachable child/cloud leaf links now,
	// rather than waiting for the next tick.
	Reconcile(ctx context.Context) error
	// GatewayNameTaken reports whether a local (non-cloud) gateway is
	// already using name.
	GatewayNameTaken(ctx context.Context, name string) (bool, error)

	// DockerStream runs docker with output copied to w (an op's log).
	DockerStream(ctx context.Context, w io.Writer, args ...string) error
	// DockerStreamEnv is DockerStream with extra process-environment
	// variables (e.g. "PROJECT=foo").
	DockerStreamEnv(ctx context.Context, w io.Writer, extraEnv []string, args ...string) error
}

// Assignment is the subset of a workspace gateway's assignment the jobs
// package needs - main's own sealed wsAssignment, copied out since that
// type isn't exported to other packages.
type Assignment struct {
	Name   string
	Source string // github | local
	Repo   string
	Branch string
	Token  string
	Dir    string // where the workspace is mounted in this gateway's own container
}
