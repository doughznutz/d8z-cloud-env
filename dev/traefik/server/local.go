package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LocalService is one of the root project's own long-running non-gateway
// services: the dev tools (deightz, the ollama gateway, gopher, the otel
// collector). The deploy tooling isn't here - it's one-shot jobs (jobs.go).
// Unlike a gateway, these live in our own compose project (selfProject) and
// are plain start/stop, no nesting.
type LocalService struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Profile string `json:"profile"`
}

var localServices = []LocalService{
	{Name: "deightz", Label: "Deightz IDE", Profile: "local"},
	{Name: "ollama-gatewai", Label: "Ollama gateway", Profile: "local"},
	{Name: "gopher", Label: "Gopher test runner", Profile: "local"},
	{Name: "otel-collector", Label: "OTel collector", Profile: "local"},
}

func localService(name string) (LocalService, bool) {
	for _, s := range localServices {
		if s.Name == name {
			return s, true
		}
	}
	return LocalService{}, false
}

// StartLocal brings up a local service. workspace, if non-empty and not
// "self" (the console's "this repo" sentinel), becomes PROJECT for this
// invocation only - e.g. deightz's DEFAULT_WORKSPACE and its project volume
// mount are ${PROJECT} substitutions compose resolves from the process
// env/--env-file at `up` time, not a container env var, so it has to ride
// along on the exec rather than live in any compose file.
func (m *Manager) StartLocal(name, workspace string) error {
	svc, ok := localService(name)
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	var projectEnv []string
	if workspace != "" && workspace != "self" {
		if err := safeProjectName(workspace); err != nil {
			return err
		}
		projectEnv = []string{"PROJECT=" + workspace}
	}
	o, err := m.projectOp("svc:"+name, "starting", "building / pulling images and starting "+name)
	if err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		m.finishProject("svc:"+name, o, m.runLocal(ctx, func(project string) error {
			args := []string{"compose", "-p", project, "-f", m.cfg.RootComposeFile, "--profile", svc.Profile, "up", "-d", "--build", svc.Name}
			return upWithRetryEnv(ctx, o.Log, "", projectEnv, args)
		}))
	}()
	return nil
}

func (m *Manager) StopLocal(name string) error {
	svc, ok := localService(name)
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	o, err := m.projectOp("svc:"+name, "stopping", "stopping "+name)
	if err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		m.finishProject("svc:"+name, o, m.runLocal(ctx, func(project string) error {
			return dockerStreamHost(ctx, o.Log, "", "compose", "-p", project, "-f", m.cfg.RootComposeFile, "stop", svc.Name)
		}))
	}()
	return nil
}

func (m *Manager) runLocal(ctx context.Context, fn func(project string) error) error {
	project := m.selfProject(ctx)
	if project == "" {
		return fmt.Errorf("can't determine our own compose project")
	}
	return fn(project)
}

// LocalOps reports in-flight start/stop state for local services, by their
// plain name (without the "svc:" op-map prefix), so /api/self can surface
// "starting"/"stopping"/"error" before (or without) a container to show it.
func (m *Manager) LocalOps() map[string]map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]map[string]string{}
	for key, o := range m.projOps {
		name, ok := strings.CutPrefix(key, "svc:")
		if !ok {
			continue
		}
		v := map[string]string{"state": o.State}
		if o.Phase != "" {
			v["phase"] = o.Phase
		}
		if o.Err != "" {
			v["error"] = o.Err
		}
		out[name] = v
	}
	return out
}
