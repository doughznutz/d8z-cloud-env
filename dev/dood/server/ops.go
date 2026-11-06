package main

import (
	"fmt"
	"strings"
)

// Background operations that aren't gateway spawns: local service
// start/stop ("svc:<name>") and jobs ("job:<id>"). Tracked in
// Manager.projOps so the console can show progress and logs before - or
// without - any container to show it.

// safeProjectName rules out path traversal in a single directory name under
// the projects root (a local workspace, a bundle source). Deliberately
// lenient otherwise: real checkouts have underscores, dots, mixed case.
func safeProjectName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid directory name %q", name)
	}
	return nil
}

// ProjectLog is an op's log by key.
func (m *Manager) ProjectLog(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.projOps[name]; o != nil {
		return o.Log.String(), true
	}
	return "", false
}

// projectOp starts tracking a new in-flight operation, refusing if one is
// already running under this key.
func (m *Manager) projectOp(name, state, phase string) (*op, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.projOps[name]; o != nil && (o.State == "starting" || o.State == "stopping") {
		return nil, fmt.Errorf("%s is busy (%s)", strings.TrimPrefix(strings.TrimPrefix(name, "svc:"), "job:"), o.State)
	}
	o := &op{State: state, Phase: phase, Log: newRingBuf(64 << 10)}
	m.projOps[name] = o
	return o, nil
}

// finishProject records the outcome of the op started under name. On error
// the op is kept with its error; a successful job is kept as "done" so its
// output stays readable; any other successful op is dropped.
func (m *Manager) finishProject(name string, o *op, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		o.State, o.Err = "error", err.Error()
		fmt.Fprintf(o.Log, "\nERROR: %v\n", err)
		return
	}
	fmt.Fprintln(o.Log, "\ndone.")
	if strings.HasPrefix(name, "job:") {
		// A job's output is its result - keep it readable after success.
		o.State, o.Phase = "done", ""
		return
	}
	delete(m.projOps, name)
}
