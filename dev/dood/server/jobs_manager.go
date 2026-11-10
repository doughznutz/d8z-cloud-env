package main

// Manager implements the jobs package's own Manager interface directly:
// the narrow slice of itself that package needs, kept separate from the
// rest of Manager's own (much wider) surface for easy review.

import (
	"context"
	"io"
	"time"

	"d8z/gw/jobs"
)

// WorkspaceName is this gateway's assigned workspace name ("" on a manager
// gateway).
func (m *Manager) WorkspaceName() string { return m.cfg.WorkspaceName }

// ProjectsRoot is ~/projects on the host ("" if this gateway has none
// mounted).
func (m *Manager) ProjectsRoot() string { return m.cfg.ProjectsRoot }

// GatewayID is this gateway's own GW_ID.
func (m *Manager) GatewayID() string { return m.cfg.ID }

// Assignment is this workspace gateway's assignment from its parent, or
// nil (not a workspace gateway, or not assigned yet) - the jobs package's
// own copy of the fields it needs from our sealed wsAssignment.
func (m *Manager) Assignment() *jobs.Assignment {
	a := m.assignment()
	if a == nil {
		return nil
	}
	return &jobs.Assignment{Name: a.Name, Source: a.Source, Repo: a.Repo, Branch: a.Branch, Token: a.Token, Dir: a.Dir}
}

// NatsAvailable reports whether the embedded NATS server is running.
func (m *Manager) NatsAvailable() bool { return m.nats != nil }

// NatsPing pings gateway id over NATS, returning its reply.
func (m *Manager) NatsPing(id, text string, timeout time.Duration) (string, error) {
	return m.nats.Ping(id, text, timeout)
}

// GatewayNameTaken reports whether a local (non-cloud) gateway is already
// using name.
func (m *Manager) GatewayNameTaken(ctx context.Context, name string) (bool, error) {
	gws, err := childGateways(ctx, m.cfg.ID)
	if err != nil {
		return false, err
	}
	for _, g := range gws {
		if g.Config.Labels[labelName] == name {
			return true, nil
		}
	}
	return false, nil
}

// DockerStream runs docker with output copied to w (an op's log).
func (m *Manager) DockerStream(ctx context.Context, w io.Writer, args ...string) error {
	return dockerStream(ctx, w, args...)
}

// DockerStreamEnv is DockerStream with extra process-environment
// variables (e.g. "PROJECT=foo").
func (m *Manager) DockerStreamEnv(ctx context.Context, w io.Writer, extraEnv []string, args ...string) error {
	return dockerStreamHostEnv(ctx, w, "", extraEnv, args...)
}
