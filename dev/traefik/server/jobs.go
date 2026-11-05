package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// JobArg is one named argument a job takes. Env, if set, names the .env
// variable that supplies its default when left blank (falling back to
// Fallback); Default is that resolved value, filled in for the console.
type JobArg struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Required bool   `json:"required,omitempty"`
	Env      string `json:"-"`
	Fallback string `json:"-"`
	Default  string `json:"default,omitempty"`
}

// JobSpec is one one-shot action against an external system. GitHub and
// Docker Hub run natively in gw-server (github.go, dockerhub.go: REST APIs
// plus the git/docker CLIs already in this image); GCP still runs in the
// dev/cloud container, which carries the gcloud SDK. Unlike a gateway or
// a local service, a job has no running/stopped state - it fires once and
// reports done/error.
type JobSpec struct {
	Group       string   `json:"group"` // github | docker | cloud
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Args        []JobArg `json:"args,omitempty"`
	Dangerous   bool     `json:"dangerous,omitempty"` // a real external write - console confirms first
	UsesProject bool     `json:"-"`                   // reads the selected project's files
	Workspace   bool     `json:"workspace,omitempty"` // repository work: runs only in a workspace's own gateway

	run func(ctx context.Context, j *jobRun) error
}

// jobRun is everything one invocation gets: its log, its resolved args and
// the project it targets.
type jobRun struct {
	m       *Manager
	spec    JobSpec
	log     io.Writer
	args    map[string]string
	project string // project name under ProjectsRoot
	dir     string // its path ("" when this gateway has no projects root)
}

func (j *jobRun) arg(name string) string { return j.args[name] }

// projectDir is for jobs that need the project's files on disk.
func (j *jobRun) projectDir() (string, error) {
	if j.dir == "" {
		return "", fmt.Errorf("no project directory: project jobs only run on the root gateway (GW_PROJECTS_ROOT)")
	}
	if st, err := os.Stat(j.dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("project %q not found at %s", j.project, j.dir)
	}
	return j.dir, nil
}

func (j *jobRun) logf(format string, a ...any) { fmt.Fprintf(j.log, format+"\n", a...) }

var (
	ghRepoArg     = JobArg{Name: "repo", Label: "repo (owner/name)"}
	ghVisArg      = JobArg{Name: "visibility", Label: "public | private", Fallback: "private"}
	dhRepoArg     = JobArg{Name: "repo", Label: "repository", Env: "DOCKER_HUB_REPO"}
	dhTagArg      = JobArg{Name: "tag", Label: "tag", Env: "DOCKER_HUB_IMAGE", Fallback: "latest"}
	dhVisArg      = JobArg{Name: "visibility", Label: "public | private", Fallback: "private"}
	instanceArg   = JobArg{Name: "instance_name", Label: "instance name", Required: true}
	gcpProjectArg = JobArg{Name: "project_id", Label: "project id", Required: true}
)

var jobs = []JobSpec{
	// GitHub (github.go): repository work, so these run only in a
	// workspace's own gateway (Workspace tab), acting on that workspace with
	// its own repo and token.
	{Group: "github", Name: "pull", Label: "Pull from GitHub (clone if empty, else fast-forward)",
		Args: []JobArg{{Name: "branch", Label: "branch (blank: workspace's)"}}, UsesProject: true, Workspace: true, run: ghPull},
	{Group: "github", Name: "diff_repo", Label: "Show what a push would change vs the default branch",
		Args: []JobArg{ghRepoArg}, UsesProject: true, Workspace: true, run: ghDiffRepo},
	{Group: "github", Name: "push_branch", Label: "Push to a branch (a GitHub workspace commits here first)",
		Args: []JobArg{{Name: "branch", Label: "branch (blank: auto-update-<time>)"}, ghRepoArg}, UsesProject: true, Workspace: true, Dangerous: true, run: ghPushBranch},
	{Group: "github", Name: "create_pull_request", Label: "Open a pull request",
		Args: []JobArg{{Name: "branch", Label: "branch", Required: true}, {Name: "title", Label: "title"}, ghRepoArg}, UsesProject: true, Workspace: true, Dangerous: true, run: ghCreatePR},
	{Group: "github", Name: "check_repo", Label: "Check repo exists",
		Args: []JobArg{ghRepoArg}, UsesProject: true, Workspace: true, run: ghCheckRepo},
	{Group: "github", Name: "create_repo", Label: "Create repo (with an initial README)",
		Args: []JobArg{ghRepoArg, ghVisArg}, UsesProject: true, Workspace: true, Dangerous: true, run: ghCreateRepo},
	{Group: "github", Name: "rename_branch", Label: "Rename a branch",
		Args: []JobArg{{Name: "old_name", Label: "old name", Required: true}, {Name: "new_name", Label: "new name", Required: true}, ghRepoArg}, UsesProject: true, Workspace: true, Dangerous: true, run: ghRenameBranch},

	// Docker Hub (dockerhub.go). Images are DOCKER_HUB_USERNAME/<repo>:<tag>.
	{Group: "docker", Name: "check_repo", Label: "Check repo exists",
		Args: []JobArg{dhRepoArg}, run: dhCheckRepo},
	{Group: "docker", Name: "create_repo", Label: "Create repo",
		Args: []JobArg{dhRepoArg, dhVisArg}, Dangerous: true, run: dhCreateRepo},
	{Group: "docker", Name: "list_repos", Label: "List repos", run: dhListRepos},
	{Group: "docker", Name: "list_tags", Label: "List tags",
		Args: []JobArg{dhRepoArg}, run: dhListTags},
	{Group: "docker", Name: "build_and_push", Label: "Build the project's image & push",
		Args:        []JobArg{{Name: "dockerfile", Label: "dockerfile", Env: "DOCKER_HUB_DOCKERFILE", Fallback: "Dockerfile"}, dhRepoArg, dhTagArg},
		UsesProject: true, Dangerous: true, run: dhBuildAndPush},
	{Group: "docker", Name: "pull", Label: "Pull image to this daemon",
		Args: []JobArg{dhRepoArg, dhTagArg}, run: dhPull},

	// GCP (dev/cloud container) - GCE instance lifecycle is the building
	// block for eventually launching gateways on cloud VMs.
	{Group: "cloud", Name: "list", Label: "List instances", run: cloudJob},
	{Group: "cloud", Name: "create", Label: "Create instance", Args: []JobArg{instanceArg}, Dangerous: true, run: cloudJob},
	{Group: "cloud", Name: "start", Label: "Start instance", Args: []JobArg{instanceArg}, run: cloudJob},
	{Group: "cloud", Name: "stop", Label: "Stop instance", Args: []JobArg{instanceArg}, run: cloudJob},
	{Group: "cloud", Name: "delete", Label: "Delete instance", Args: []JobArg{instanceArg}, Dangerous: true, run: cloudJob},
	{Group: "cloud", Name: "list-projects", Label: "List GCP projects", run: cloudJob},
	{Group: "cloud", Name: "describe-project", Label: "Describe GCP project", Args: []JobArg{gcpProjectArg}, run: cloudJob},
	{Group: "cloud", Name: "set-project", Label: "Set active GCP project", Args: []JobArg{gcpProjectArg}, run: cloudJob},
}

func (a JobArg) resolvedDefault() string {
	if a.Env != "" {
		if v := strings.TrimSpace(os.Getenv(a.Env)); v != "" {
			return v
		}
	}
	return a.Fallback
}

// JobCatalog is the catalog as the console sees it, defaults filled in.
func JobCatalog() []JobSpec {
	out := make([]JobSpec, len(jobs))
	for i, j := range jobs {
		j.Args = append([]JobArg(nil), j.Args...)
		for k := range j.Args {
			j.Args[k].Default = j.Args[k].resolvedDefault()
		}
		out[i] = j
	}
	return out
}

func findJob(group, name string) (JobSpec, bool) {
	for _, j := range jobs {
		if j.Group == group && j.Name == name {
			return j, true
		}
	}
	return JobSpec{}, false
}

// defaultProject is the .env's PROJECT - what "this repo" means on the host.
func defaultProject() string { return strings.TrimSpace(os.Getenv("PROJECT")) }

// RunJob validates against the catalog and runs the job in the background.
// One run per job at a time; its id is "<group>/<name>".
func (m *Manager) RunJob(group, name, project string, args map[string]string) (string, error) {
	spec, ok := findJob(group, name)
	if !ok {
		return "", fmt.Errorf("unknown job %s/%s", group, name)
	}
	resolved := map[string]string{}
	for _, a := range spec.Args {
		v := strings.TrimSpace(args[a.Name])
		if v == "" {
			v = a.resolvedDefault()
		}
		if a.Required && v == "" {
			return "", fmt.Errorf("%s/%s: %q is required", group, name, a.Name)
		}
		resolved[a.Name] = v
	}
	for k := range args {
		if _, ok := resolved[k]; !ok {
			return "", fmt.Errorf("%s/%s: unknown argument %q", group, name, k)
		}
	}
	ws := m.assignment()
	if spec.Workspace && ws == nil {
		if m.cfg.WorkspaceName != "" {
			return "", fmt.Errorf("workspace %q hasn't been assigned by the parent gateway yet", m.cfg.WorkspaceName)
		}
		return "", fmt.Errorf("%s/%s is repository work: run it from a workspace's own gateway (root console → Workspaces → start its gateway)", group, name)
	}
	if project == "" || project == "self" {
		project = defaultProject()
	}
	j := &jobRun{m: m, spec: spec, args: resolved, project: project}
	if ws != nil {
		// A workspace gateway works on exactly one tree: its workspace.
		j.project, j.dir = ws.Name, ws.Dir
		if resolved["repo"] == "" && ws.Repo != "" {
			resolved["repo"] = ws.Repo // every github job, including pull (which takes no repo arg)
		}
		if _, ok := resolved["branch"]; ok && resolved["branch"] == "" && spec.Name == "pull" {
			resolved["branch"] = ws.Branch
		}
	} else if spec.UsesProject {
		if project == "" {
			return "", fmt.Errorf("no project selected (and no PROJECT in .env)")
		}
		if err := safeProjectName(project); err != nil {
			return "", err
		}
		if m.cfg.ProjectsRoot != "" {
			j.dir = filepath.Join(m.cfg.ProjectsRoot, project)
		}
	}
	id := group + "/" + name
	o, err := m.projectOp("job:"+id, "starting", "running "+id)
	if err != nil {
		return "", err
	}
	j.log = o.Log
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if ws != nil && spec.Group == "github" {
			ctx = withWorkspaceToken(ctx, ws.Name, ws.Token)
		}
		if ws != nil {
			j.logf("workspace: %s (%s)", ws.Name, ws.Dir)
		} else if spec.UsesProject {
			j.logf("project: %s", j.project)
		}
		m.finishProject("job:"+id, o, spec.run(ctx, j))
	}()
	return id, nil
}

// cloudJob runs a GCP command in the dev/cloud container:
// `docker compose run --rm --build --no-TTY cloud <name> [args...]`.
func cloudJob(ctx context.Context, j *jobRun) error {
	project := j.m.selfProject(ctx)
	if project == "" {
		return fmt.Errorf("can't determine our own compose project")
	}
	a := []string{"compose", "-p", project, "-f", j.m.cfg.RootComposeFile, "--profile", "deploy",
		"run", "--rm", "--build", "--no-TTY", "cloud", j.spec.Name}
	for _, arg := range j.spec.Args {
		a = append(a, j.args[arg.Name])
	}
	return dockerStreamHost(ctx, j.log, "", a...)
}

func (m *Manager) JobLog(id string) (string, bool) {
	return m.ProjectLog("job:" + id)
}

// JobRuns reports in-flight/last state per job id, the same idea as
// LocalOps but for the "job:" op-map namespace. A run that finished cleanly
// is kept (state "done") so its log stays readable.
func (m *Manager) JobRuns() map[string]map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]map[string]string{}
	for key, o := range m.projOps {
		id, ok := strings.CutPrefix(key, "job:")
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
		out[id] = v
	}
	return out
}
