package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Project is one source tree under Config.ProjectsRoot, as the console sees
// it: either an ordinary compose stack, a sibling that has adopted this
// repo's gateway-bundle convention (and so can be spawned as a nested
// gateway via SpawnRequest.Project), or both.
type Project struct {
	Name       string          `json:"name"`
	HasCompose bool            `json:"hasCompose"` // docker-compose.yaml at its root
	HasBundle  bool            `json:"hasBundle"`  // dev/traefik/bundle/compose.yaml - spawnable as a gateway
	Services   []ServiceStatus `json:"services"`
	State      string          `json:"state,omitempty"` // starting | stopping | error, while an op is in flight
	Phase      string          `json:"phase,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type ProjectSource struct {
	Kind string `json:"kind"` // "self" (this repo's own baked-in /self) | "git"
	URL  string `json:"url,omitempty"`
	Ref  string `json:"ref,omitempty"`
}

// projectNameRE is deliberately strict: it's only for names we mint
// ourselves (CreateProject). Starting/stopping an *existing* directory
// (StartProject/StopProject/bundleFile) has to tolerate whatever a real
// checkout is actually called - underscores, dots, mixed case - so those
// use safeProjectName instead, which just rules out path traversal.
var projectNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func safeProjectName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid project name %q", name)
	}
	return nil
}

// sanitizeComposeProject turns an arbitrary directory name into a valid
// compose `-p` project id (lowercase letters, digits, "-", "_").
func sanitizeComposeProject(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (m *Manager) projectPath(name string) string { return filepath.Join(m.cfg.ProjectsRoot, name) }

func (m *Manager) projectComposeProject(name string) string {
	return "d8zproj-" + sanitizeComposeProject(name)
}

func (m *Manager) ListProjects(ctx context.Context) ([]Project, error) {
	if m.cfg.ProjectsRoot == "" {
		return nil, fmt.Errorf("no projects root configured")
	}
	entries, err := os.ReadDir(m.cfg.ProjectsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	svcs, _ := projectServices(ctx, "")
	out := make([]Project, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		p := Project{Name: name}
		if _, err := os.Stat(filepath.Join(m.projectPath(name), "docker-compose.yaml")); err == nil {
			p.HasCompose = true
		}
		if _, err := os.Stat(filepath.Join(m.projectPath(name), "dev/traefik/bundle/compose.yaml")); err == nil {
			p.HasBundle = true
		}
		p.Services = append([]ServiceStatus{}, svcs[m.projectComposeProject(name)]...)
		out = append(out, p)
	}
	m.mu.Lock()
	for name, o := range m.projOps {
		if strings.HasPrefix(name, "svc:") || strings.HasPrefix(name, "job:") {
			continue // a local-service or job op, not a project (see local.go / jobs.go)
		}
		switch o.State {
		case "starting", "stopping":
		case "error":
		default:
			continue
		}
		found := false
		for i := range out {
			if out[i].Name == name {
				out[i].State, out[i].Phase, out[i].Error = o.State, o.Phase, o.Err
				found = true
				break
			}
		}
		if !found {
			out = append(out, Project{Name: name, State: o.State, Phase: o.Phase, Error: o.Err})
		}
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Manager) ProjectLog(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.projOps[name]; o != nil {
		return o.Log.String(), true
	}
	return "", false
}

// projectOp starts tracking a new in-flight project operation, refusing if
// one is already running for this name.
func (m *Manager) projectOp(name, state, phase string) (*op, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.projOps[name]; o != nil && (o.State == "starting" || o.State == "stopping") {
		return nil, fmt.Errorf("project %q is busy (%s)", name, o.State)
	}
	o := &op{State: state, Phase: phase, Log: newRingBuf(64 << 10)}
	m.projOps[name] = o
	return o, nil
}

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

// CreateProject materializes a new directory under ProjectsRoot: a mounted
// project (one that already exists there) always wins over this - it's only
// ever used to provision one that doesn't exist yet.
func (m *Manager) CreateProject(name string, src ProjectSource) error {
	if m.cfg.ProjectsRoot == "" {
		return fmt.Errorf("no projects root configured")
	}
	if !projectNameRE.MatchString(name) {
		return fmt.Errorf("name must match %s", projectNameRE)
	}
	if src.Kind != "self" && src.Kind != "git" {
		return fmt.Errorf("source kind must be self or git")
	}
	if src.Kind == "git" && strings.TrimSpace(src.URL) == "" {
		return fmt.Errorf("git source needs a url")
	}
	dest := m.projectPath(name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("project %q already exists", name)
	}
	o, err := m.projectOp(name, "starting", "provisioning")
	if err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		m.finishProject(name, o, provisionProject(ctx, o.Log, dest, src))
	}()
	return nil
}

func provisionProject(ctx context.Context, w io.Writer, dest string, src ProjectSource) error {
	if src.Kind == "git" {
		args := []string{"clone", "--depth", "1"}
		if src.Ref != "" {
			args = append(args, "--branch", src.Ref)
		}
		args = append(args, src.URL, dest)
		return runStream(ctx, w, "git", args...)
	}
	// "self": a plain copy of the image's own baked-in checkout.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return runStream(ctx, w, "cp", "-a", "/self/.", dest)
}

// StartProject brings an ordinary project's own docker-compose.yaml up on
// our daemon, as compose project d8zproj-<name>. Unlike a gateway, this
// doesn't nest a child Traefik - it's just "run this stack", shown in the
// console as a plain container list.
func (m *Manager) StartProject(name string) error {
	if err := safeProjectName(name); err != nil {
		return err
	}
	composeFile := filepath.Join(m.projectPath(name), "docker-compose.yaml")
	if _, err := os.Stat(composeFile); err != nil {
		return fmt.Errorf("project %q has no docker-compose.yaml", name)
	}
	o, err := m.projectOp(name, "starting", "building / pulling images and starting containers")
	if err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		args := []string{"compose", "-p", m.projectComposeProject(name), "-f", composeFile, "up", "-d"}
		m.finishProject(name, o, upWithRetry(ctx, o.Log, "", args))
	}()
	return nil
}

func (m *Manager) StopProject(name string) error {
	if err := safeProjectName(name); err != nil {
		return err
	}
	composeFile := filepath.Join(m.projectPath(name), "docker-compose.yaml")
	o, err := m.projectOp(name, "stopping", "stopping containers")
	if err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := dockerStreamHost(ctx, o.Log, "", "compose", "-p", m.projectComposeProject(name), "-f", composeFile, "down")
		m.finishProject(name, o, err)
	}()
	return nil
}

// runStream is dockerStreamHost's non-docker counterpart, for the plain
// commands (git, cp) project provisioning needs.
func runStream(ctx context.Context, w io.Writer, name string, args ...string) error {
	fmt.Fprintf(w, "$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	return nil
}
