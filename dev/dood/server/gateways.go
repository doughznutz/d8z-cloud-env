package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Gateway is one child bundle as the console sees it.
type Gateway struct {
	Name       string          `json:"name"`
	ID         string          `json:"id"`
	Project    string          `json:"project"`
	Prefix     string          `json:"prefix"`
	Path       string          `json:"path"` // relative to this console, e.g. "gw/abc/"
	Docker     string          `json:"docker"`
	Profiles   []string        `json:"profiles"`
	State      string          `json:"state"` // starting | running | stopping | exited | error | ...
	Error      string          `json:"error,omitempty"`
	Upstream   string          `json:"upstream,omitempty"`
	Ready      bool            `json:"ready"` // reachable through our own Traefik
	Phase      string          `json:"phase,omitempty"`
	Services   []ServiceStatus `json:"services"`
	Container  string          `json:"container,omitempty"`
	SrcProject string          `json:"srcProject,omitempty"` // sibling project it was spawned from ("" = this repo)
	Workspace  string          `json:"workspace,omitempty"`  // workspace it serves
}

// SpawnRequest is everything needed to start a child gateway. It's also
// recorded in the child's labels, so the same request can be rebuilt later
// (childSpec) to restart it.
type SpawnRequest struct {
	Name      string   `json:"name"`
	Profiles  []string `json:"profiles"`
	Docker    string   `json:"docker"`    // host | dind: the child's own mode
	Project   string   `json:"project"`   // sibling project to spawn from instead of this repo; "" = this repo
	Workspace string   `json:"workspace"` // workspace (from workspace.yaml) this gateway serves; "" = none
	Branch    string   `json:"branch"`    // saved config branch to start from (cfgtree.go); "" = none
}

// nameRE is a valid child gateway name: no "-" or ".", so IDs and NATS
// subjects built from names stay unambiguous.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

// errExists is returned when spawning a gateway whose name is taken (or busy).
var errExists = errors.New("gateway already exists")

// op tracks an in-flight (or failed) spawn/stop so the console can show
// progress before - or after - the containers themselves exist.
type op struct {
	State string       // starting | stopping | error | done
	Phase string       // human-readable step while starting/stopping
	Err   string       // the failure, when State is "error"
	Req   SpawnRequest // what was asked for (spawns only)
	Log   *ringBuf     // the op's output, for the console
}

// Manager is gw-server's state: our config, in-flight ops, the NATS hub,
// the workspace store and assignment, and our children's requests. All of
// the gateway's behaviour hangs off it.
type Manager struct {
	cfg      Config
	self     string                       // our container id (docker sets the hostname to it)
	mu       sync.Mutex                   // guards ops, projOps, requests and every op's fields
	ops      map[string]*op               // spawn/stop ops, by child gateway name
	projOps  map[string]*op               // same shape, keyed by project name - kept separate so a gateway and a project can share a name
	recMu    sync.Mutex                   // serializes Reconcile
	ready    sync.Map                     // name -> upstream it was last seen ready at
	selfProj atomic.Value                 // our compose project name, once found (selfProject)
	ws       *wsStore                     // workspace.yaml
	assign   atomic.Pointer[wsAssignment] // a workspace gateway's assignment, once its parent has sent it
	nats     *NatsHub                     // nil if the embedded NATS server failed to start
	requests map[string]*ParentRequest    // our children's, by name (requests.go); under mu
}

// NewManager builds the Manager and starts the embedded NATS hub. A NATS
// failure is logged, not fatal: the gateway runs without it (m.nats nil).
func NewManager(cfg Config) *Manager {
	self, _ := os.Hostname()
	m := &Manager{cfg: cfg, self: self, ops: map[string]*op{}, projOps: map[string]*op{}, ws: &wsStore{file: cfg.WorkspaceFile}}
	hub, err := startNatsHub(cfg)
	if err != nil {
		log.Printf("nats: %v", err)
	}
	m.nats = hub
	onConfigWrite = m.configChanged
	return m
}

// childID is child name's GW_ID: its bare name under the root, else our ID
// and its name dash-joined.
func (m *Manager) childID(name string) string {
	if m.cfg.ID == "root" {
		return name
	}
	return m.cfg.ID + "-" + name
}

// project is child name's compose project name.
func (m *Manager) project(name string) string { return "d8zgw-" + m.childID(name) }

// prefix is the external URL prefix child name is mounted at.
func (m *Manager) prefix(name string) string { return m.cfg.Prefix + "/gw/" + name }

// routeFile is child name's Traefik file-provider route.
func (m *Manager) routeFile(name string) string {
	return filepath.Join(m.cfg.RoutesDir, "gw-"+name+".yml")
}

// overrideFile is child name's generated compose override.
func (m *Manager) overrideFile(name string) string {
	return filepath.Join(m.cfg.StateDir, m.project(name)+".override.yaml")
}

// bundleFile resolves which bundle compose file a spawn should use: this
// repo's own (srcProject == "") or a sibling project's, if it has adopted
// the same dev/dood/bundle/compose.yaml convention.
func (m *Manager) bundleFile(srcProject string) (string, error) {
	if srcProject == "" {
		return m.cfg.BundleFile, nil
	}
	if m.cfg.ProjectsRoot == "" {
		return "", fmt.Errorf("no projects root configured")
	}
	if err := safeProjectName(srcProject); err != nil {
		return "", err
	}
	f := filepath.Join(m.cfg.ProjectsRoot, srcProject, "dev/dood/bundle/compose.yaml")
	if _, err := os.Stat(f); err != nil {
		return "", fmt.Errorf("project %q has no dev/dood/bundle/compose.yaml: %w", srcProject, err)
	}
	return f, nil
}

// upstream is how *this* Traefik reaches the child, over the child's
// gwlink network: the child's Traefik itself (host), or its dind sidecar,
// on which the Traefik inside publishes 8080 (dind).
func (m *Manager) upstream(c container) string {
	return "http://" + c.name() + ":8080"
}

// innerHost is the docker API of a dind child's sidecar, reachable over
// its gwlink network.
func innerHost(project string) string {
	return "tcp://" + project + "-dind-1:2375"
}

// List is every child gateway, sorted by name: running ones from docker,
// plus any still starting, stopping or failed from our ops. A running child
// reports "starting" until probe sees it answer through our Traefik.
func (m *Manager) List(ctx context.Context) ([]Gateway, error) {
	cs, err := childGateways(ctx, m.cfg.ID)
	svcs, _ := projectServices(ctx, "")
	byName := map[string]*Gateway{}
	for _, c := range cs {
		l := c.Config.Labels
		g := &Gateway{
			Name: l[labelName], ID: l[labelID], Project: l[labelProject], Prefix: l[labelPrefix],
			Docker: l[labelDocker], State: c.State.Status, Container: c.name(),
			SrcProject: l[labelSrcProject], Workspace: l[labelWorkspace],
		}
		if l[labelProfiles] != "" {
			g.Profiles = strings.Split(l[labelProfiles], ",")
		}
		if g.State == "running" {
			g.Upstream = m.upstream(c)
		}
		byName[g.Name] = g
	}
	m.mu.Lock()
	for name, o := range m.ops {
		g := byName[name]
		if g == nil {
			if o.State == "done" {
				continue
			}
			g = &Gateway{Name: name, ID: m.childID(name), Project: m.project(name), Prefix: m.prefix(name),
				Docker: o.Req.Docker, Profiles: resolveProfiles(o.Req.Profiles, o.Req.Docker), SrcProject: o.Req.Project,
				Workspace: o.Req.Workspace}
			byName[name] = g
		}
		switch o.State {
		case "starting", "stopping":
			g.State, g.Phase = o.State, o.Phase
		case "error":
			g.State, g.Error = "error", o.Err
		}
	}
	m.mu.Unlock()
	out := make([]Gateway, 0, len(byName))
	for _, g := range byName {
		g.Path = "gw/" + g.Name + "/"
		// A running container isn't a usable gateway until our Traefik has
		// loaded its route and the child answers; report "starting" until
		// then so the console doesn't load (and keep) a 404 in its iframe.
		g.Services = append([]ServiceStatus{}, svcs[g.Project]...)
		if g.Docker == "dind" && g.State == "running" {
			ictx, cancel := context.WithTimeout(ctx, 2*time.Second)
			if inner, err := projectServices(ictx, innerHost(g.Project)); err == nil {
				g.Services = append(g.Services, inner[g.Project]...)
			}
			cancel()
		}
		if g.State == "running" {
			if g.Ready = m.probe(ctx, g); !g.Ready {
				g.State = "starting"
				g.Phase = "waiting for the gateway to answer"
			}
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}

// probe asks our own Traefik for the child's API: anything but a routing
// failure (404 = route not loaded yet, 502/503 = child not up) means the
// cascade link works. The root answers 401 here - that's fine, it proves
// the router exists. Once seen ready at an upstream it stays ready.
func (m *Manager) probe(ctx context.Context, g *Gateway) bool {
	if g.Upstream == "" {
		return false
	}
	if up, ok := m.ready.Load(g.Name); ok && up == g.Upstream {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	url := "http://127.0.0.1:" + getenv("GW_PORT", "80") + g.Prefix + "/api/self"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotFound, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return false
	}
	m.ready.Store(g.Name, g.Upstream)
	return true
}

// Log is the output of child name's current or last spawn/stop op.
func (m *Manager) Log(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.ops[name]; o != nil {
		return o.Log.String(), true
	}
	return "", false
}

// Spawn validates req and starts the child gateway in the background,
// tracked as an op; errExists if the name is taken or busy.
func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) error {
	if !nameRE.MatchString(req.Name) {
		return fmt.Errorf("name must match %s", nameRE)
	}
	if req.Docker == "" {
		req.Docker = "host"
	}
	if req.Docker != "host" && req.Docker != "dind" {
		return fmt.Errorf("docker must be host or dind")
	}
	if _, err := m.bundleFile(req.Project); err != nil {
		return err
	}
	if req.Branch != "" {
		if _, err := m.loadBranch(req.Branch); err != nil {
			return err
		}
	}
	if req.Workspace != "" {
		if _, _, _, err := m.workspaceFor(req.Workspace); err != nil {
			return err
		}
	}
	cs, err := childGateways(ctx, m.cfg.ID)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if c.Config.Labels[labelName] == req.Name {
			return errExists
		}
	}
	m.mu.Lock()
	if o := m.ops[req.Name]; o != nil && (o.State == "starting" || o.State == "stopping") {
		m.mu.Unlock()
		return errExists
	}
	o := &op{State: "starting", Req: req, Log: newRingBuf(64 << 10)}
	m.ops[req.Name] = o
	m.mu.Unlock()
	go m.runSpawn(req, o)
	return nil
}

// finish records the outcome of child name's op: an error is kept for the
// console; on success the op is dropped (removeOp) or marked done.
func (m *Manager) finish(name string, o *op, err error, removeOp bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case err != nil:
		o.State, o.Err = "error", err.Error()
		fmt.Fprintf(o.Log, "\nERROR: %v\n", err)
	case removeOp:
		delete(m.ops, name)
	default:
		o.State = "done"
		fmt.Fprintln(o.Log, "\ndone.")
	}
}

// setPhase updates the step an op shows the console.
func (m *Manager) setPhase(o *op, phase string) {
	m.mu.Lock()
	o.Phase = phase
	m.mu.Unlock()
}

// runSpawn is Spawn's background half: bring the bundle up, then
// Reconcile so its route and NATS leaf link are attached.
func (m *Manager) runSpawn(req SpawnRequest, o *op) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	if err := m.spawn(ctx, req, o); err != nil {
		m.finish(req.Name, o, err, false)
		return
	}
	m.setPhase(o, "attaching route")
	if err := m.Reconcile(ctx); err != nil {
		fmt.Fprintf(o.Log, "reconcile: %v\n", err)
	}
	m.finish(req.Name, o, nil, false)
}

// spawn brings a bundle up. host: the bundle runs on our daemon, its
// Traefik on gwlink. dind: only the bundle's dind sidecar runs on our
// daemon (and is the only thing on gwlink); we then drive that daemon to
// run the whole bundle inside it.
func (m *Manager) spawn(ctx context.Context, req SpawnRequest, o *op) error {
	project := m.project(req.Name)
	profs := resolveProfiles(req.Profiles, req.Docker)
	bundleFile, err := m.bundleFile(req.Project)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.cfg.StateDir, 0o755); err != nil {
		return err
	}
	compose := func(ovr string, profiles ...string) []string {
		a := []string{"compose", "-p", project, "--env-file", m.cfg.EnvFile, "-f", bundleFile, "-f", ovr}
		for _, p := range profiles {
			a = append(a, "--profile", p)
		}
		return a
	}
	write := func(suffix, content string) (string, error) {
		f := filepath.Join(m.cfg.StateDir, project+suffix)
		return f, os.WriteFile(f, []byte(content), 0o644)
	}
	labels := m.childLabels(req, profs)
	// Same project this bundle was spawned from becomes its own PROJECT -
	// compose resolves ${PROJECT} (deightz's workspace mount/default) from
	// the process env at `up` time, not from a container env var, so this
	// has to ride along on the exec rather than live in the override YAML.
	var projectEnv []string
	if req.Project != "" {
		projectEnv = []string{"PROJECT=" + req.Project}
	}
	ws, err := m.spawnWorkspace(ctx, o, req)
	if err != nil {
		return err
	}
	if ws != nil {
		projectEnv = []string{"PROJECT=" + wsMountName(ws.Workspace)}
	}

	if req.Docker != "dind" {
		ovr, err := write(".override.yaml", m.renderOverride(req, profs, labels, false, ws))
		if err != nil {
			return err
		}
		m.setPhase(o, "building / pulling images and starting containers")
		return upWithRetryEnv(ctx, o.Log, "", projectEnv, append(compose(ovr, profs...), "up", "-d"))
	}

	// 1. The sidecar, on our daemon. With a workspace, it mounts only that
	// workspace (not the whole projects root), at the path the bundle inside
	// bind-mounts it from.
	side, err := write(".sidecar.yaml", renderSidecar(labels, ws))
	if err != nil {
		return err
	}
	m.setPhase(o, "starting the dind sidecar")
	if err := upWithRetry(ctx, o.Log, "", append(compose(side, "dind"), "up", "-d", "--wait", "dind")); err != nil {
		return err
	}
	// 2. Reach its API over gwlink.
	m.attach(ctx, project)
	inner := innerHost(project)
	m.setPhase(o, "waiting for the sidecar's docker daemon")
	var verr error
	for i := 0; i < 60; i++ {
		if _, verr = dockerOut(ctx, "-H", inner, "version"); verr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if verr != nil {
		return fmt.Errorf("sidecar daemon unreachable: %w", verr)
	}
	// 3. The bundle, inside it.
	if ws != nil {
		// Inside the sidecar the workspace is just a directory on its
		// filesystem: bind it from there.
		ws.Mount = wsMount{Type: "bind", Source: ws.Mount.Target, Target: ws.Mount.Target}
	}
	ovr, err := write(".override.yaml", m.renderOverride(req, profs, nil, true, ws))
	if err != nil {
		return err
	}
	args := compose(ovr, profs...)
	m.seedImages(ctx, o, inner, append(append([]string{}, args...), "config", "--images"))
	m.setPhase(o, "building / pulling images and starting the bundle inside the sidecar")
	return upWithRetryEnv(ctx, o.Log, inner, projectEnv, append(args, "up", "-d"))
}

// upWithRetry runs a compose command (against host, "" = ours) and retries
// once: fresh dind daemons occasionally fail BuildKit's registry lookups.
func upWithRetry(ctx context.Context, w *ringBuf, host string, args []string) error {
	return upWithRetryEnv(ctx, w, host, nil, args)
}

// upWithRetryEnv is upWithRetry with extra process-environment variables
// (see dockerStreamHostEnv).
func upWithRetryEnv(ctx context.Context, w *ringBuf, host string, extraEnv []string, args []string) error {
	err := dockerStreamHostEnv(ctx, w, host, extraEnv, args...)
	if err != nil {
		fmt.Fprintf(w, "\n%v - retrying once\n", err)
		err = dockerStreamHostEnv(ctx, w, host, extraEnv, args...)
	}
	return err
}

// childLabels are the d8z.gw.* labels put on a child's entry container.
func (m *Manager) childLabels(req SpawnRequest, profs []string) [][2]string {
	return [][2]string{
		{labelParent, m.cfg.ID}, {labelID, m.childID(req.Name)}, {labelName, req.Name},
		{labelPrefix, m.prefix(req.Name)}, {labelDocker, req.Docker},
		{labelProfiles, strings.Join(profs, ",")}, {labelEntry, "true"},
		{labelSrcProject, req.Project}, {labelWorkspace, req.Workspace}, {labelBranch, req.Branch},
	}
}

// Stop removes a child. Its config changes first: merged into ours, saved as
// a branch (config == "" or "branch"), or dropped ("discard").
func (m *Manager) Stop(name string, config string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("bad name")
	}
	switch config {
	case "":
		config = "branch"
	case "branch", "merge", "discard":
	default:
		return fmt.Errorf("config must be merge, branch or discard")
	}
	m.mu.Lock()
	o := m.ops[name]
	if o != nil && (o.State == "starting" || o.State == "stopping") {
		m.mu.Unlock()
		return fmt.Errorf("gateway is busy (%s)", o.State)
	}
	if o == nil {
		o = &op{}
	}
	o.State, o.Err, o.Log, o.Phase = "stopping", "", newRingBuf(64<<10), "removing bundle and its descendants"
	m.ops[name] = o
	m.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if config != "discard" {
			m.setPhase(o, "collecting its config changes")
			m.takeChildConfig(o.Log, name, config)
			m.setPhase(o, "removing bundle and its descendants")
		}
		os.Remove(m.routeFile(name))
		m.ready.Delete(name)
		err := m.stopTree(ctx, o.Log, m.childID(name), m.project(name))
		os.Remove(m.overrideFile(name))
		os.Remove(filepath.Join(m.cfg.StateDir, m.project(name)+".sidecar.yaml"))
		m.Reconcile(ctx)
		m.finish(name, o, err, true)
	}()
	return nil
}

// stopTree removes a bundle after its descendants. Descendants of a host
// gateway share our daemon and are found by label; a dind gateway's live
// inside its sidecar and go away with it.
func (m *Manager) stopTree(ctx context.Context, w *ringBuf, id, project string) error {
	kids, err := childGateways(ctx, id)
	if err != nil {
		return err
	}
	for _, k := range kids {
		l := k.Config.Labels
		if l[labelDocker] == "dind" {
			// Everything below a dind gateway lives inside its sidecar.
			fmt.Fprintf(w, "removing %s (and everything inside its dind)\n", l[labelProject])
			if err := removeProject(ctx, w, l[labelProject]); err != nil {
				fmt.Fprintf(w, "descendant %s: %v\n", l[labelID], err)
			}
			continue
		}
		if err := m.stopTree(ctx, w, l[labelID], l[labelProject]); err != nil {
			fmt.Fprintf(w, "descendant %s: %v\n", l[labelID], err)
		}
	}
	fmt.Fprintf(w, "removing %s\n", project)
	return removeProject(ctx, w, project)
}

// Reconcile makes the route files (and, in host mode, our gwlink
// attachments) match the running children. Runs after every change and
// periodically, so routes survive restarts of either side.
func (m *Manager) Reconcile(ctx context.Context) error {
	m.recMu.Lock()
	defer m.recMu.Unlock()
	cs, err := childGateways(ctx, m.cfg.ID)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	leafChildren := map[string]string{} // name -> container, for the NATS leaf remotes below
	for _, c := range cs {
		name := c.Config.Labels[labelName]
		if c.State.Status != "running" || !nameRE.MatchString(name) {
			continue
		}
		m.attach(ctx, c.Config.Labels[labelProject])
		up := m.upstream(c)
		want[m.routeFile(name)] = true
		writeIfChanged(m.routeFile(name), m.renderRoute(name, up))
		leafChildren[name] = childLeafURL(c.name(), m.cfg.NatsLeafPort)
	}
	// Same direction, same trigger as the HTTP routes above: once we're on a
	// running child's gwlink, dial into its NATS leaf listener too. Cloud
	// gateways we deployed (deployment.go) are dialled the same way, over
	// the internet.
	if m.nats != nil {
		for name, u := range m.cloudLeafURLs() {
			leafChildren["cloud:"+name] = u
		}
		m.nats.setRemotes(leafChildren)
	}
	m.ready.Range(func(k, _ any) bool {
		if !want[m.routeFile(k.(string))] {
			m.ready.Delete(k) // re-probe if it comes back
		}
		return true
	})
	files, _ := filepath.Glob(filepath.Join(m.cfg.RoutesDir, "gw-*.yml"))
	for _, f := range files {
		if !want[f] {
			os.Remove(f)
		}
	}
	return nil
}

// attach puts our container on a child's gwlink network (idempotent).
func (m *Manager) attach(ctx context.Context, project string) {
	link := project + "_gwlink"
	nets, err := dockerOut(ctx, "inspect", "-f", "{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}", m.self)
	if err == nil {
		for _, n := range strings.Fields(nets) {
			if n == link {
				return
			}
		}
	}
	if _, err := dockerOut(ctx, "network", "connect", link, m.self); err != nil {
		log.Printf("attach %s: %v", link, err)
	}
}

// writeIfChanged atomically replaces path with content unless it already
// holds exactly that (errors are ignored: the next Reconcile retries).
func writeIfChanged(path, content string) {
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err == nil {
		os.Rename(tmp, path) // atomic, so Traefik's watcher never sees half a file
	}
}

// renderRoute is the Traefik dynamic config routing child name's prefix
// to upstream.
func (m *Manager) renderRoute(name, upstream string) string {
	p := m.prefix(name)
	return fmt.Sprintf(`# Generated by gw-server for child gateway %q - do not edit.
http:
  routers:
    gw-%[2]s:
      rule: "Path(`+"`%[3]s`"+`) || PathPrefix(`+"`%[3]s/`"+`)"
      entryPoints: [web]
      middlewares: [gw-guard, add-trailing-slash]
      service: gw-%[2]s
  services:
    gw-%[2]s:
      loadBalancer:
        servers:
          - url: %[4]q
`, m.childID(name), name, p, upstream)
}

// dozzlePath is the (absolute) path of the dozzle watching our daemon:
// inherited if we share it with our parent, else our own.
func (m *Manager) dozzlePath() string {
	if m.cfg.DozzlePath != "" {
		return m.cfg.DozzlePath
	}
	return m.cfg.Prefix + "/dozzle/"
}

// yq quotes s for YAML (a JSON string is a valid YAML scalar).
func yq(s string) string { b, _ := json.Marshal(s); return string(b) }

// renderSidecar labels a dind child's sidecar as the gateway's entry and,
// for a workspace gateway, narrows its mounts to just that workspace.
func renderSidecar(labels [][2]string, ws *wsAssignment) string {
	var b bytes.Buffer
	b.WriteString("# Generated by gw-server - do not edit.\nservices:\n  dind:\n    labels:\n")
	for _, kv := range labels {
		fmt.Fprintf(&b, "      %s: %s\n", kv[0], yq(kv[1]))
	}
	if ws != nil {
		b.WriteString("    volumes: !override\n")
		writeMount(&b, ws.Mount)
		b.WriteString("      - dind-data:/var/lib/docker\n")
		writeExternalVolume(&b, ws.Mount)
	}
	return b.String()
}

// writeMount renders one long-form compose volume entry (6-space list indent).
func writeMount(b *bytes.Buffer, mt wsMount) {
	fmt.Fprintf(b, "      - type: %s\n        source: %s\n        target: %s\n", mt.Type, yq(mt.Source), yq(mt.Target))
}

// writeExternalVolume declares a workspace volume as external: it's created
// outside compose (spawnWorkspace) precisely so that stopping the gateway -
// removeProject deletes volumes by compose project label - never deletes
// the workspace.
func writeExternalVolume(b *bytes.Buffer, mt wsMount) {
	if mt.Type == "volume" {
		fmt.Fprintf(b, "volumes:\n  %s:\n    external: true\n", mt.Source)
	}
}

// spawnWorkspace resolves a spawn's workspace into how it's mounted on our
// daemon, creating a github workspace's volume if it doesn't exist yet. The
// child itself is only told the workspace's name (and its key); the details
// it asks for over NATS once we're linked (wsnats.go).
func (m *Manager) spawnWorkspace(ctx context.Context, o *op, req SpawnRequest) (*wsAssignment, error) {
	if req.Workspace == "" {
		return nil, nil
	}
	w, mt, token, err := m.workspaceFor(req.Workspace)
	if err != nil {
		return nil, err
	}
	if mt.Type == "volume" {
		if _, err := dockerOut(ctx, "volume", "inspect", mt.Source); err != nil {
			fmt.Fprintf(o.Log, "creating workspace volume %s\n", mt.Source)
			if _, err := dockerOut(ctx, "volume", "create", "--label", "d8z.workspace="+w.Name, mt.Source); err != nil {
				return nil, err
			}
		}
	} else {
		// Our own workspace we see at its mount; any other at its host
		// path (the projects root is mounted at the same path).
		dir := mt.Source
		if m.cfg.WorkspaceName != "" && w.Name == m.cfg.WorkspaceName {
			dir = mt.Target
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("workspace %q: %s is not a directory", w.Name, dir)
		}
	}
	a := &wsAssignment{Workspace: w, Dir: mt.Target, Mount: mt}
	a.Token = token
	a.TokenEnv = ""
	return a, nil
}

// workspaceFor resolves a workspace a child of ours may work on, as it's
// mounted on our daemon, with its token: one from our own workspace.yaml, or
// the workspace our parent gave us, handed on down (keys only ever flow
// down the tree).
func (m *Manager) workspaceFor(name string) (Workspace, wsMount, string, error) {
	if m.cfg.WorkspaceName != "" && name == m.cfg.WorkspaceName {
		a := m.assignment()
		if a == nil {
			return Workspace{}, wsMount{}, "", fmt.Errorf("workspace %s isn't assigned to this gateway yet", name)
		}
		return a.Workspace, a.Mount, a.Token, nil
	}
	w, err := m.ws.get(name)
	if err != nil {
		return Workspace{}, wsMount{}, "", err
	}
	return w, m.hostMount(w), wsToken(w), nil
}

// renderOverride is the per-child compose override layered on
// bundle/compose.yaml: identity, prefixes and (host mode) labels. With
// inDind the bundle runs inside its sidecar, so Traefik publishes 8080 on
// it instead of joining our gwlink.
func (m *Manager) renderOverride(req SpawnRequest, profs []string, labels [][2]string, inDind bool, ws *wsAssignment) string {
	id, prefix := m.childID(req.Name), m.prefix(req.Name)
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Generated by gw-server (parent %s) - do not edit.\nservices:\n", m.cfg.ID)
	fmt.Fprintf(&b, "  dood:\n    environment:\n")
	// Every sidebar-capable service, not just the ones started with it: the
	// console lists only those actually in its bundle, so one started later
	// on demand (og, gopher, carol) shows up too.
	isolated, dozzle, svcs := "false", "", uiServices(allProfileNames())
	if inDind {
		isolated = "true"
	} else {
		// Shares our daemon: point its console at that daemon's dozzle.
		dozzle = m.dozzlePath()
	}
	for _, kv := range [][2]string{
		{"GW_ROLE", "child"}, {"GW_ID", id}, {"GW_PREFIX", prefix}, {"GW_PORT", "8080"},
		{"GW_ISOLATED", isolated}, {"GW_DOZZLE_PATH", dozzle}, {"GW_SERVICES", strings.Join(svcs, ",")},
		// Every child's key to what we give it (config, and a workspace):
		// sealed for it alone (wsnats.go).
		{"GW_WORKSPACE_KEY", m.childKey(id)},
	} {
		fmt.Fprintf(&b, "      %s: %s\n", kv[0], yq(kv[1]))
	}
	if ws != nil {
		// The workspace gateway: told only which workspace it serves and
		// the key to unseal the rest (repo, mount, token), which it fetches
		// from us over NATS; and given that workspace read/write on top of
		// what every gateway's dood has (the projects root, its own
		// workspace.yaml) - so it manages workspaces of its own, just like us.
		for _, kv := range [][2]string{
			{"GW_WORKSPACE_NAME", ws.Name},
			// The shared .env's token is the root's, not this workspace's:
			// keep it out (the workspace's own arrives sealed over NATS).
			{"GITHUB_TOKEN", ""},
		} {
			fmt.Fprintf(&b, "      %s: %s\n", kv[0], yq(kv[1]))
		}
		b.WriteString("    volumes: !override\n      - /var/run/docker.sock:/var/run/docker.sock\n" +
			"      - /home/${USER}/projects:/home/${USER}/projects\n      - d8z-config:/etc/d8z\n      - d8z-nats:/var/lib/d8zgw-nats\n")
		writeMount(&b, ws.Mount)
	}
	if len(labels) > 0 {
		fmt.Fprintf(&b, "    labels:\n")
		for _, kv := range labels {
			fmt.Fprintf(&b, "      %s: %s\n", kv[0], yq(kv[1]))
		}
	}
	if inDind {
		// HTTP and the NATS leaf listener, both reached by us at the sidecar.
		fmt.Fprintf(&b, "    ports: [\"8080:8080\", \"%[1]d:%[1]d\"]\n", m.cfg.NatsLeafPort)
	}
	fmt.Fprintf(&b, "  dozzle:\n    environment:\n      DOZZLE_BASE: %s\n", yq(prefix+"/dozzle"))
	fmt.Fprintf(&b, "  og:\n    environment:\n      OG_BASE_PATH: %s\n", yq(strings.TrimPrefix(prefix+"/og", "/")))
	fmt.Fprintf(&b, "  gopher:\n    environment:\n      GOPHER_BASE_PATH: %s\n", yq(strings.TrimPrefix(prefix+"/gopher", "/")))
	fmt.Fprintf(&b, "  carol:\n    environment:\n      CAROL_BASE_PATH: %s\n", yq(strings.TrimPrefix(prefix+"/carol", "/")))
	if ws != nil {
		// Its Deightz edits the same workspace.
		b.WriteString("  deightz:\n    environment:\n      GITHUB_TOKEN: \"\"\n    volumes: !override\n      - /var/run/docker.sock:/var/run/docker.sock\n")
		writeMount(&b, ws.Mount)
		writeExternalVolume(&b, ws.Mount)
	}
	if inDind {
		// Port publishing is silently dropped on internal networks.
		fmt.Fprintf(&b, "networks:\n  gwlink:\n    internal: false\n")
	}
	return b.String()
}

// seedImages copies the bundle's images from our daemon into a dind
// child's sidecar (target), so it doesn't rebuild/pull everything from
// scratch. Best effort: anything not copied is built or pulled by
// `compose up`.
func (m *Manager) seedImages(ctx context.Context, o *op, target string, configArgs []string) {
	list, err := dockerOut(ctx, configArgs...)
	if err != nil {
		fmt.Fprintf(o.Log, "seed: %v\n", err)
		return
	}
	imgs := strings.Fields(list)
	for i, img := range imgs {
		if _, err := dockerOut(ctx, "-H", target, "image", "inspect", img); err == nil {
			continue // already there
		}
		sz, err := dockerOut(ctx, "image", "inspect", "-f", "{{.Size}}", img)
		if err != nil {
			continue // we don't have it either; compose will build/pull
		}
		total, _ := strconv.ParseInt(sz, 10, 64)
		label := fmt.Sprintf("copying image %s (%d/%d) into the sidecar", img, i+1, len(imgs))
		fmt.Fprintf(o.Log, "%s, ~%s\n", label, human(total))
		m.setPhase(o, label)
		save := exec.CommandContext(ctx, "docker", "save", img)
		load := exec.CommandContext(ctx, "docker", "-H", target, "load")
		pipe, err := save.StdoutPipe()
		if err != nil {
			continue
		}
		cr := &countingReader{r: pipe}
		load.Stdin, load.Stdout, load.Stderr, save.Stderr = cr, o.Log, o.Log, o.Log
		if err := save.Start(); err != nil {
			fmt.Fprintf(o.Log, "seed %s: %v\n", img, err)
			continue
		}
		done := make(chan struct{})
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-t.C:
					m.setPhase(o, fmt.Sprintf("%s: %s / ~%s", label, human(cr.n.Load()), human(total)))
				}
			}
		}()
		if err := load.Run(); err != nil {
			fmt.Fprintf(o.Log, "seed %s: %v\n", img, err)
		}
		close(done)
		save.Wait()
	}
}

// countingReader counts the bytes read through it, readable concurrently.
type countingReader struct {
	r io.Reader
	n atomic.Int64
}

// Read reads from the underlying reader and adds to the count.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// human formats a byte count as KB, MB or GB.
func human(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", b>>10)
	}
}

// ringBuf keeps the tail of an op's output.
type ringBuf struct {
	mu  sync.Mutex
	max int
	buf []byte
}

// newRingBuf makes a ringBuf keeping the last max bytes.
func newRingBuf(max int) *ringBuf { return &ringBuf{max: max} }

// Write appends p, dropping the oldest bytes beyond max. It never fails.
func (r *ringBuf) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if over := len(r.buf) - r.max; over > 0 {
		r.buf = r.buf[over:]
	}
	return len(p), nil
}

// String is the buffered tail ("" for a nil ringBuf).
func (r *ringBuf) String() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// selfProject is the compose project our own container belongs to
// (cached once found).
func (m *Manager) selfProject(ctx context.Context) string {
	if p, ok := m.selfProj.Load().(string); ok && p != "" {
		return p
	}
	p, err := dockerOut(ctx, "inspect", "-f", `{{index .Config.Labels "`+labelProject+`"}}`, m.self)
	if err != nil {
		return ""
	}
	m.selfProj.Store(p)
	return p
}

// Bundle reports our own bundle's services.
func (m *Manager) Bundle(ctx context.Context) ([]ServiceStatus, error) {
	p := m.selfProject(ctx)
	if p == "" {
		return []ServiceStatus{}, fmt.Errorf("can't determine our compose project")
	}
	all, err := projectServices(ctx, "")
	if err != nil {
		return []ServiceStatus{}, err
	}
	return append([]ServiceStatus{}, all[p]...), nil
}

// BundleLogs is the tail of one of our own bundle's service logs.
func (m *Manager) BundleLogs(ctx context.Context, service string) (string, error) {
	p := m.selfProject(ctx)
	if p == "" {
		return "", fmt.Errorf("can't determine our compose project")
	}
	return serviceLogs(ctx, "", p, service)
}

// ChildLogs reads a child bundle service's log: from our daemon, or for a
// dind child (except its sidecar) from inside the sidecar.
func (m *Manager) ChildLogs(ctx context.Context, name, service string) (string, error) {
	if !nameRE.MatchString(name) {
		return "", fmt.Errorf("bad name")
	}
	host := ""
	if service != "dind" {
		cs, _ := childGateways(ctx, m.cfg.ID)
		for _, c := range cs {
			if c.Config.Labels[labelName] == name && c.Config.Labels[labelDocker] == "dind" {
				host = innerHost(m.project(name))
			}
		}
	}
	return serviceLogs(ctx, host, m.project(name), service)
}

// takeChildConfig merges a child's config changes into ours, or saves them
// as a branch; returns the branch id if one was saved. Best effort: a child
// that doesn't answer can't hand anything over, and that's logged.
func (m *Manager) takeChildConfig(w io.Writer, name, mode string) string {
	ops, err := m.ChildConfigDiff(name)
	if err != nil {
		fmt.Fprintf(w, "config: couldn't get its changes (%v) - nothing merged or saved\n", err)
		return ""
	}
	if len(ops) == 0 {
		fmt.Fprintln(w, "config: no changes")
		return ""
	}
	for _, o := range ops {
		fmt.Fprintf(w, "config: %s\n", o.summary())
	}
	if mode == "merge" {
		if err := m.mergeConfig(ops); err != nil {
			fmt.Fprintf(w, "config: merge failed (%v) - saving it as a branch instead\n", err)
		} else {
			fmt.Fprintln(w, "config: merged")
			return ""
		}
	}
	id, err := m.saveBranch(name, ops)
	if err != nil {
		fmt.Fprintf(w, "config: couldn't save the branch: %v\n", err)
		return ""
	}
	fmt.Fprintf(w, "config: saved as branch %s\n", id)
	return id
}
