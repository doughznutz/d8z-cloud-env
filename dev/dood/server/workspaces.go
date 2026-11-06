package main

// Workspaces replace the old single PROJECT. A workspace is one source tree
// a gateway works on: a GitHub repository (pulled into its own docker volume,
// d8zws-<name>) or a local directory under the projects root (bind-mounted).
//
// The root gateway is the *manager*: it owns workspace.yaml (which holds the
// GitHub tokens, so it lives outside every repository and is mounted in at
// /etc/d8z), lists/edits workspaces and spawns one child gateway per
// workspace. It never mounts any workspace's content itself.
//
// A child spawned for a workspace is that workspace's *gateway*: its
// gw-server and its Deightz both mount the workspace read/write, and all
// repository work (pull, diff, push, PRs - the github/* jobs) happens there.
// It starts knowing only its workspace's name; the rest - repo, branch,
// where it's mounted, the token - it asks its parent for over NATS
// (wsnats.go), sealed with a key only it and the parent hold.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"

	"gopkg.in/yaml.v3"
)

// Workspace is one entry of workspace.yaml.
type Workspace struct {
	Name     string `yaml:"name" json:"name"`                              // lowercase; also names its volume (d8zws-<name>)
	Source   string `yaml:"source" json:"source"`                          // github | local
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`          // local: directory under the projects root
	Repo     string `yaml:"repo,omitempty" json:"repo,omitempty"`          // owner/name on github.com (optional for local)
	Branch   string `yaml:"branch,omitempty" json:"branch,omitempty"`      // branch to pull (blank: the repo's default)
	Token    string `yaml:"token,omitempty" json:"-"`                      // GitHub token - never sent to the console
	TokenEnv string `yaml:"token_env,omitempty" json:"tokenEnv,omitempty"` // or the env var holding it
	Gateway  string `yaml:"gateway,omitempty" json:"gateway,omitempty"`    // child gateway name (blank: derived from name)

	// The workspace's gateway: which services it runs beside its dood
	// (bundle profiles; dozzle follows the one-per-daemon rule) and where.
	// Services is nil when unset (the default, [deightz]) and an empty list
	// for "dood only" - so a pointer, which yaml can tell apart.
	Services *[]string `yaml:"services,omitempty" json:"-"`
	Docker   string    `yaml:"docker,omitempty" json:"docker"` // host | dind (blank: host)
}

// wsServices is a workspace's gateway services with the default applied.
func wsServices(w Workspace) []string {
	if w.Services == nil {
		return []string{"deightz"}
	}
	return append([]string{}, (*w.Services)...)
}

// wsDocker is a workspace's gateway docker mode with the default applied.
func wsDocker(w Workspace) string {
	if w.Docker == "" {
		return "host"
	}
	return w.Docker
}

// wsFile is the whole of workspace.yaml.
type wsFile struct {
	Workspaces []Workspace `yaml:"workspaces"`
}

// wsMount is how a workspace is mounted, as seen by the docker daemon the
// mounting container runs on.
type wsMount struct {
	Type   string `json:"type"`   // volume | bind
	Source string `json:"source"` // volume name, or a path on that daemon's filesystem
	Target string `json:"target"` // path inside the container
}

// wsAssignment is what a workspace gateway is told about its workspace by
// its parent (over NATS, sealed - see wsnats.go). Token travels inside it.
type wsAssignment struct {
	Workspace
	Token string  `json:"token,omitempty"` // shadows Workspace.Token (json:"-") so it survives the sealed hop
	Dir   string  `json:"dir"`             // where it's mounted in the gateway's own container
	Mount wsMount `json:"mount"`           // the same mount, for its Deightz
}

// Validation patterns for a workspace's name, its owner/name repo, and
// environment variable names (token_env and the like).
var (
	wsNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	wsRepoRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}/[A-Za-z0-9._-]{1,100}$`)
	wsEnvRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// wsStore is workspace.yaml. It's re-read on every access (it's tiny), so a
// hand edit on the host shows up without a restart.
type wsStore struct {
	mu   sync.Mutex
	file string
}

// defaultWorkspaces is what a manager works with before any workspace.yaml
// exists: the .env's own PROJECT as a local workspace, pushing to
// GITHUB_ORG/GITHUB_REPO with GITHUB_TOKEN - i.e. exactly what the console
// did before workspaces. It's written to disk on the first edit.
func defaultWorkspaces() []Workspace {
	p := defaultProject()
	if p == "" {
		return []Workspace{}
	}
	ws := Workspace{Name: strings.ToLower(p), Source: "local", Path: p, TokenEnv: "GITHUB_TOKEN"}
	if !wsNameRE.MatchString(ws.Name) {
		ws.Name = "default"
	}
	owner := strings.TrimSpace(os.Getenv("GITHUB_ORG"))
	if owner == "" {
		owner = strings.TrimSpace(os.Getenv("GITHUB_USER"))
	}
	if repo := strings.TrimSpace(os.Getenv("GITHUB_REPO")); repo != "" && owner != "" {
		ws.Repo = owner + "/" + repo
	}
	return []Workspace{ws}
}

// load returns the workspaces and whether they came from the file.
func (s *wsStore) load() ([]Workspace, bool, error) {
	b, err := os.ReadFile(s.file)
	if errors.Is(err, os.ErrNotExist) {
		return defaultWorkspaces(), false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var f wsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, true, fmt.Errorf("%s: %w", s.file, err)
	}
	if f.Workspaces == nil {
		f.Workspaces = []Workspace{}
	}
	return f.Workspaces, true, nil
}

// get looks up one workspace by name.
func (s *wsStore) get(name string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, _, err := s.load()
	if err != nil {
		return Workspace{}, err
	}
	for _, w := range list {
		if w.Name == name {
			return w, nil
		}
	}
	return Workspace{}, fmt.Errorf("no workspace %q", name)
}

// wsFileHeader is written at the top of every workspace.yaml we save, as a
// worked example of the format.
const wsFileHeader = `# D8Z workspaces - managed by the gateway console (Workspaces tab).
# This file holds GitHub tokens: keep it outside every repository.
#
#   - name: my-repo            # lowercase; also names its volume (d8zws-<name>)
#     source: github           # github: pulled into a docker volume
#     repo: owner/name
#     branch: main             # optional: blank pulls the default branch
#     token: ghp_...           # or token_env: GITHUB_TOKEN (read from .env)
#   - name: scratch
#     source: local            # local: a directory under ~/projects
#     path: scratch
#     repo: owner/scratch      # optional: where push/diff/PRs go
#     services: [deightz, og]  # its gateway's services (default: deightz)
#     docker: dind             # host (default) | dind: its own isolated daemon
`

// save writes the file atomically, keeping an existing file's owner (so a
// hand-managed file on the host stays editable by its user) and mode 0600.
func (s *wsStore) save(list []Workspace) error {
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(wsFile{Workspaces: list}); err != nil {
		return err
	}
	content := append([]byte(wsFileHeader+"\n"), buf.Bytes()...)
	return writeConfigFile(s.file, content)
}

// writeConfigFile writes a config file (workspace.yaml, deployment.yaml, a
// key) atomically with mode 0600, keeping an existing file's owner - or else
// its directory's - so a file on the host stays editable by its user.
func writeConfigFile(file string, content []byte) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	uid, gid := -1, -1
	if st, err := os.Stat(file); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(sys.Uid), int(sys.Gid)
		}
	} else if st, err := os.Stat(dir); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(sys.Uid), int(sys.Gid)
		}
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	os.Chown(tmp, uid, gid)
	if err := os.Rename(tmp, file); err != nil {
		// The file itself is bind-mounted (not its directory): can't
		// replace it, so rewrite it in place.
		os.Remove(tmp)
		if err := os.WriteFile(file, content, 0o600); err != nil {
			return err
		}
	}
	if onConfigWrite != nil {
		onConfigWrite()
	}
	return nil
}

// onConfigWrite runs after every config file write: our children rebase
// onto the new config (Manager.configChanged).
var onConfigWrite func()

// update loads the workspaces, applies fn and saves the result, under the
// store's lock.
func (s *wsStore) update(fn func([]Workspace) ([]Workspace, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, _, err := s.load()
	if err != nil {
		return err
	}
	list, err = fn(list)
	if err != nil {
		return err
	}
	return s.save(list)
}

// validateWorkspace trims and checks w in place, and normalizes it: a
// pasted clone URL becomes owner/name, and default values are cleared so
// the file stays minimal.
func (m *Manager) validateWorkspace(w *Workspace) error {
	w.Name = strings.TrimSpace(w.Name)
	w.Path = strings.TrimSpace(w.Path)
	w.Repo = strings.TrimSpace(w.Repo)
	w.Branch = strings.TrimSpace(w.Branch)
	w.TokenEnv = strings.TrimSpace(w.TokenEnv)
	w.Gateway = strings.TrimSpace(w.Gateway)
	if r, err := parseGHRepo(w.Repo); w.Repo != "" && err == nil && strings.Contains(w.Repo, "github.com") {
		w.Repo = r.String() // accept a pasted clone URL
	}
	if !wsNameRE.MatchString(w.Name) {
		return fmt.Errorf("name must match %s", wsNameRE)
	}
	if w.Repo != "" && !wsRepoRE.MatchString(w.Repo) {
		return fmt.Errorf("repo must be owner/name")
	}
	if w.Branch != "" && (!branchRE.MatchString(w.Branch) || strings.Contains(w.Branch, "..") || strings.HasPrefix(w.Branch, "-")) {
		return fmt.Errorf("invalid branch %q", w.Branch)
	}
	if w.TokenEnv != "" && !wsEnvRE.MatchString(w.TokenEnv) {
		return fmt.Errorf("token_env must be an environment variable name")
	}
	if w.Gateway != "" && !nameRE.MatchString(w.Gateway) {
		return fmt.Errorf("gateway name must match %s", nameRE)
	}
	if w.Docker != "" && w.Docker != "host" && w.Docker != "dind" {
		return fmt.Errorf("docker must be host or dind")
	}
	if w.Docker == "host" {
		w.Docker = "" // the default: keep the file minimal
	}
	switch w.Source {
	case "github":
		w.Path = ""
		if w.Repo == "" {
			return fmt.Errorf("a github workspace needs a repo (owner/name)")
		}
	case "local":
		if err := safeProjectName(w.Path); err != nil {
			return fmt.Errorf("local path must be one directory name under the projects root")
		}
		if m.cfg.ProjectsRoot == "" {
			return fmt.Errorf("this gateway has no projects root, so it can't use local workspaces")
		}
		if st, err := os.Stat(filepath.Join(m.cfg.ProjectsRoot, w.Path)); err != nil || !st.IsDir() {
			return fmt.Errorf("%s is not a directory under the projects root", w.Path)
		}
	default:
		return fmt.Errorf("source must be github or local")
	}
	if w.Services != nil {
		svcs := []string{}
		for _, n := range *w.Services {
			ok := false
			for _, p := range profiles {
				ok = ok || (p.Name == n && n != "dozzle") // dozzle is decided by docker mode
			}
			if !ok {
				return fmt.Errorf("unknown service %q", n)
			}
			svcs = append(svcs, n)
		}
		if len(svcs) == 1 && svcs[0] == "deightz" {
			w.Services = nil // the default: keep the file minimal
		} else {
			w.Services = &svcs
		}
	}
	return nil
}

// wsGatewayName is the child gateway serving a workspace.
func wsGatewayName(w Workspace) string {
	if w.Gateway != "" {
		return w.Gateway
	}
	var b strings.Builder
	for _, r := range w.Name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	n := strings.TrimLeft(b.String(), "0123456789")
	if n == "" {
		n = "ws"
	}
	if len(n) > 16 {
		n = n[:16]
	}
	return n
}

// wsMountName is the directory name the workspace appears as under
// /home/$USER/projects inside its containers.
func wsMountName(w Workspace) string {
	if w.Source == "local" {
		return w.Path
	}
	return w.Name
}

// wsVolume is a github workspace's docker volume.
func wsVolume(w Workspace) string { return "d8zws-" + w.Name }

// wsTarget is where a workspace is mounted inside its containers.
func wsTarget(w Workspace) string {
	return "/home/" + getenv("USER", "abc") + "/projects/" + wsMountName(w)
}

// hostMount is the workspace's mount on *our* daemon.
func (m *Manager) hostMount(w Workspace) wsMount {
	if w.Source == "local" {
		// The root bind-mounts the projects root at the same path as on
		// the host, so this path is valid on the daemon too.
		return wsMount{Type: "bind", Source: filepath.Join(m.cfg.ProjectsRoot, w.Path), Target: wsTarget(w)}
	}
	return wsMount{Type: "volume", Source: wsVolume(w), Target: wsTarget(w)}
}

// wsToken is the workspace's GitHub token ("" if it has none).
func wsToken(w Workspace) string {
	if t := strings.TrimSpace(w.Token); t != "" {
		return t
	}
	if w.TokenEnv != "" {
		return strings.TrimSpace(os.Getenv(w.TokenEnv))
	}
	return ""
}

// assignment is this workspace gateway's workspace, or nil (not a workspace
// gateway, or its parent hasn't answered yet).
func (m *Manager) assignment() *wsAssignment { return m.assign.Load() }

// prepareWorkspaceDir makes a fresh volume's root writable by the editor
// user: docker creates it root-owned, but git and Deightz run as PUID.
func prepareWorkspaceDir(a *wsAssignment) {
	if a == nil {
		return
	}
	st, err := os.Stat(a.Dir)
	if err != nil {
		return
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid == 0 {
		if entries, _ := os.ReadDir(a.Dir); len(entries) == 0 {
			os.Chown(a.Dir, editorUID, editorGID)
		}
	}
}

// editorUID and editorGID own workspace files: Deightz's PUID/PGID.
const editorUID, editorGID = 1000, 1000

// wsGitState describes a checkout from its .git directory alone (no git
// process): whether it's there and which branch is checked out.
func wsGitState(dir string) (present, isGit bool, head string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, false, ""
	}
	present = len(entries) > 0
	b, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		return present, false, ""
	}
	h := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(h, "ref: refs/heads/"); ok {
		return present, true, ref
	}
	if len(h) > 12 {
		h = h[:12]
	}
	return present, true, h
}

// wsView is one workspace as the console sees it.
type wsView struct {
	Workspace
	Services   []string `json:"services"` // with the default applied
	TokenSet   bool     `json:"tokenSet"`
	Gateway    string   `json:"gateway"`
	GatewayRef *Gateway `json:"gw,omitempty"`
}

// listWorkspaces is the same on every gateway: the workspaces it manages
// (its own workspace.yaml) and, if its parent gave it one, the workspace it
// serves itself.
func (m *Manager) listWorkspaces(ctx context.Context) (map[string]any, error) {
	m.ws.mu.Lock()
	list, persisted, err := m.ws.load()
	m.ws.mu.Unlock()
	resp := map[string]any{"file": m.ws.file, "persisted": persisted, "dirs": m.projectDirs()}
	if m.cfg.WorkspaceName != "" {
		if a := m.assignment(); a == nil {
			resp["own"] = map[string]any{"name": m.cfg.WorkspaceName, "pending": true}
		} else {
			present, isGit, head := wsGitState(a.Dir)
			resp["own"] = map[string]any{
				"name": a.Name, "source": a.Source, "path": a.Path, "repo": a.Repo, "branch": a.Branch,
				"dir": a.Dir, "tokenSet": a.Token != "", "present": present, "git": isGit, "head": head,
				"writable": syscall.Access(a.Dir, 2) == nil,
			}
		}
	}
	if err != nil {
		resp["error"] = err.Error()
		list = []Workspace{}
	}
	gws, _ := m.List(ctx)
	views := make([]wsView, 0, len(list))
	for _, w := range list {
		v := wsView{Workspace: w, TokenSet: wsToken(w) != "", Gateway: wsGatewayName(w)}
		v.Services, v.Docker = wsServices(w), wsDocker(w)
		for i := range gws {
			if gws[i].Name == v.Gateway {
				v.GatewayRef = &gws[i]
			}
		}
		views = append(views, v)
	}
	resp["workspaces"] = views
	return resp, nil
}

// projectDirs are the directories a local workspace can point at.
func (m *Manager) projectDirs() []string {
	out := []string{}
	if m.cfg.ProjectsRoot == "" {
		return out
	}
	entries, _ := os.ReadDir(m.cfg.ProjectsRoot)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// wsInput is an add/edit request: the workspace plus token handling.
type wsInput struct {
	Workspace
	Token      string    `json:"token"`      // set a new token ("" keeps the old one on edit)
	Services   *[]string `json:"services"`   // the gateway's services (absent: the default)
	ClearToken bool      `json:"clearToken"` // drop the stored token
}

// addWorkspace validates and adds a new workspace, refusing a duplicate
// name or gateway name.
func (m *Manager) addWorkspace(in wsInput) error {
	w := in.Workspace
	w.Token = strings.TrimSpace(in.Token)
	w.Services = in.Services
	if err := m.validateWorkspace(&w); err != nil {
		return err
	}
	return m.ws.update(func(list []Workspace) ([]Workspace, error) {
		for _, o := range list {
			if o.Name == w.Name {
				return nil, fmt.Errorf("workspace %q already exists", w.Name)
			}
			if wsGatewayName(o) == wsGatewayName(w) {
				return nil, fmt.Errorf("gateway name %q is already used by workspace %q - set a different gateway name", wsGatewayName(w), o.Name)
			}
		}
		return append(list, w), nil
	})
}

// editWorkspace replaces workspace name's settings. Its name can't change,
// and its stored token is kept unless a new one is given or it's cleared.
func (m *Manager) editWorkspace(name string, in wsInput) error {
	return m.ws.update(func(list []Workspace) ([]Workspace, error) {
		for i, o := range list {
			if o.Name != name {
				continue
			}
			w := in.Workspace
			w.Services = in.Services
			w.Name = o.Name // renaming would orphan its volume and gateway
			w.Token = o.Token
			if in.ClearToken {
				w.Token = ""
			}
			if t := strings.TrimSpace(in.Token); t != "" {
				w.Token = t
			}
			if err := m.validateWorkspace(&w); err != nil {
				return nil, err
			}
			list[i] = w
			return list, nil
		}
		return nil, fmt.Errorf("no workspace %q", name)
	})
}

// removeWorkspace drops it from the file. Its gateway must be stopped first;
// purge also deletes a github workspace's volume (a local directory is never
// touched).
func (m *Manager) removeWorkspace(ctx context.Context, name string, purge bool) error {
	w, err := m.ws.get(name)
	if err != nil {
		return err
	}
	gws, _ := m.List(ctx)
	for _, g := range gws {
		if g.Name == wsGatewayName(w) {
			return fmt.Errorf("stop its gateway gw/%s first", g.Name)
		}
	}
	if purge && w.Source == "github" {
		if out, err := dockerOut(ctx, "volume", "rm", wsVolume(w)); err != nil && !strings.Contains(out, "no such volume") {
			return fmt.Errorf("couldn't delete volume %s: %v", wsVolume(w), err)
		}
	}
	return m.ws.update(func(list []Workspace) ([]Workspace, error) {
		out := list[:0]
		for _, o := range list {
			if o.Name != name {
				out = append(out, o)
			}
		}
		return out, nil
	})
}

// registerWorkspaceRoutes adds the Workspaces tab's endpoints: list, add,
// edit and remove workspaces, and start a workspace's gateway.
func registerWorkspaceRoutes(mux *http.ServeMux, m *Manager) {
	mux.HandleFunc("GET /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		resp, err := m.listWorkspaces(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var in wsInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := m.addWorkspace(in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"name": in.Name})
	})
	mux.HandleFunc("PUT /api/workspaces/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in wsInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := m.editWorkspace(r.PathValue("name"), in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": r.PathValue("name")})
	})
	mux.HandleFunc("DELETE /api/workspaces/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.removeWorkspace(r.Context(), r.PathValue("name"), r.URL.Query().Get("purge") == "true"); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"state": "removed"})
	})
	// Start the workspace's own gateway (Deightz + repository manager).
	mux.HandleFunc("POST /api/workspaces/{name}/gateway", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Docker   string   `json:"docker"`
			Profiles []string `json:"profiles"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil && err.Error() != "EOF" {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ws, err := m.ws.get(r.PathValue("name"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		// The workspace's own spec, unless this one start overrides it.
		if req.Profiles == nil {
			req.Profiles = wsServices(ws)
		}
		if req.Docker == "" {
			req.Docker = wsDocker(ws)
		}
		sr := SpawnRequest{Name: wsGatewayName(ws), Profiles: req.Profiles, Docker: req.Docker, Workspace: ws.Name}
		if err := m.Spawn(r.Context(), sr); err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, errExists) {
				code = http.StatusConflict
			}
			writeErr(w, code, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"name": sr.Name, "state": "starting"})
	})
}
