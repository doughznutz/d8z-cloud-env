// Package jobs is one-shot actions against external systems: GitHub
// (github.go), Docker Hub (dockerhub.go) and GCP (gcp.go), all native (REST
// over apiclient, plus the git/docker CLIs already in the image). Docker
// Hub and GCP are driven by deployment.yaml (the deployment package); a few
// things - a workspace's own assignment, NATS, reconciling leaf links,
// running docker with output streamed to a job's log - stay with main's
// Manager, reached through the narrow Manager interface (manager.go).
package jobs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"d8z/gw/deployment"
	"d8z/gw/optracker"
)

// JobArg is one named argument a job takes. Env, if set, names the .env
// variable that supplies its default when left blank (falling back to
// Fallback); Default is that resolved value, filled in for the console.
type JobArg struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Required bool     `json:"required,omitempty"`
	Env      string   `json:"-"`
	Fallback string   `json:"-"`
	Default  string   `json:"default,omitempty"`
	From     string   `json:"-"`                 // "images" | "targets": choices come from deployment.yaml
	Choices  []string `json:"choices,omitempty"` // filled in from From; the console shows a select
}

// JobSpec is one one-shot action against an external system. Unlike a
// gateway or a local service, a job has no running/stopped state - it
// fires once and reports done/error.
type JobSpec struct {
	Group       string   `json:"group"` // github | docker | cloud
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Args        []JobArg `json:"args,omitempty"`
	Dangerous   bool     `json:"dangerous,omitempty"` // a real external write - console confirms first
	UsesProject bool     `json:"-"`                   // reads the selected project's files
	Workspace   bool     `json:"workspace,omitempty"` // repository work: runs only in a workspace's own gateway

	run func(ctx context.Context, j *jobRun) error // the job itself; its error is the run's outcome
}

// jobRun is everything one invocation gets: its catalog (and so its
// Manager, deployment store and op tracker), its log, its resolved args
// and the project it targets.
type jobRun struct {
	c       *Catalog
	spec    JobSpec
	log     io.Writer
	args    map[string]string
	project string // project name under ProjectsRoot
	dir     string // its path ("" when this gateway has no projects root)
}

// arg is the resolved value of argument name ("" if it has none).
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

// logf writes one formatted line to the run's log.
func (j *jobRun) logf(format string, a ...any) { fmt.Fprintf(j.log, format+"\n", a...) }

// Arguments shared by several jobs in the catalog.
var (
	ghRepoArg   = JobArg{Name: "repo", Label: "repo (owner/name)"}
	ghVisArg    = JobArg{Name: "visibility", Label: "public | private", Fallback: "private"}
	dhImageArg  = JobArg{Name: "image", Label: "image", Fallback: "all", From: "images"}
	dhVisArg    = JobArg{Name: "visibility", Label: "public | private", Fallback: "public"}
	cloudTgtArg = JobArg{Name: "target", Label: "target", Required: true, From: "targets"}
)

// catalog is the job catalog, in the order the console lists it.
var catalog = []JobSpec{
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

	// Docker Hub (dockerhub.go): the named images of deployment.yaml, each
	// a tag of one repository. Builds use the project's live checkout.
	{Group: "docker", Name: "build", Label: "Build image(s) locally (compose --profile dh build)",
		Args: []JobArg{dhImageArg}, UsesProject: true, run: dhBuildJob},
	{Group: "docker", Name: "push", Label: "Push image(s) to Docker Hub",
		Args: []JobArg{dhImageArg}, Dangerous: true, run: dhPushJob},
	{Group: "docker", Name: "build_and_push", Label: "Build image(s) and push",
		Args: []JobArg{dhImageArg}, UsesProject: true, Dangerous: true, run: dhBuildAndPush},
	{Group: "docker", Name: "pull", Label: "Pull image(s) to this daemon",
		Args: []JobArg{dhImageArg}, run: dhPull},
	{Group: "docker", Name: "list_tags", Label: "List the repository's tags", run: dhListTags},
	{Group: "docker", Name: "check_repo", Label: "Check the repository exists", run: dhCheckRepo},
	{Group: "docker", Name: "create_repo", Label: "Create the repository",
		Args: []JobArg{dhVisArg}, Dangerous: true, run: dhCreateRepo},
	{Group: "docker", Name: "list_repos", Label: "List the account's repositories", run: dhListRepos},

	// Cloud (gcp.go): deployment.yaml targets, each a GCE VM running a
	// profile from our Docker Hub images; reached over HTTP and NATS.
	{Group: "cloud", Name: "deploy", Label: "Deploy (create the VM, or update + restart it to pull the latest images)",
		Args: []JobArg{cloudTgtArg}, Dangerous: true, run: cloudDeploy},
	{Group: "cloud", Name: "status", Label: "Refresh status (VM, address, NATS ping)",
		Args: []JobArg{cloudTgtArg}, run: cloudStatus},
	{Group: "cloud", Name: "start", Label: "Start the VM", Args: []JobArg{cloudTgtArg}, Dangerous: true, run: cloudStart},
	{Group: "cloud", Name: "stop", Label: "Stop the VM", Args: []JobArg{cloudTgtArg}, run: cloudStop},
	{Group: "cloud", Name: "delete", Label: "Delete the VM and its firewall rule",
		Args: []JobArg{cloudTgtArg}, Dangerous: true, run: cloudDelete},
	{Group: "cloud", Name: "list", Label: "List d8z gateway VMs", run: cloudList},
}

// resolvedDefault is the arg's value when left blank: its Env variable if
// set, else its Fallback.
func (a JobArg) resolvedDefault() string {
	if a.Env != "" {
		if v := strings.TrimSpace(os.Getenv(a.Env)); v != "" {
			return v
		}
	}
	return a.Fallback
}

// Catalog is the job catalog bound to one gateway: its Manager, its
// deployment.yaml store, and its op tracker ("job:<id>" keys).
type Catalog struct {
	mgr     Manager
	deploy  *deployment.Store
	tracker *optracker.Tracker
}

// NewCatalog builds a Catalog. deploy and tracker are held directly (not
// through an interface): both are plain, dependency-free types any package
// can import.
func NewCatalog(mgr Manager, deploy *deployment.Store, tracker *optracker.Tracker) *Catalog {
	return &Catalog{mgr: mgr, deploy: deploy, tracker: tracker}
}

// argChoices are an arg's allowed values from deployment.yaml (nil: free text).
func (c *Catalog) argChoices(a JobArg) []string {
	if a.From == "" {
		return nil
	}
	out := []string{}
	d, _, err := c.deploy.Load()
	if err != nil {
		return out
	}
	switch a.From {
	case "images":
		out = append(out, "all")
		for _, im := range d.DockerHub.Images {
			out = append(out, im.Name)
		}
	case "targets":
		for _, t := range d.Targets {
			out = append(out, t.Name)
		}
	}
	return out
}

// JobCatalog is the catalog as the console sees it, defaults and choices
// filled in.
func (c *Catalog) JobCatalog() []JobSpec {
	out := make([]JobSpec, len(catalog))
	for i, j := range catalog {
		j.Args = append([]JobArg(nil), j.Args...)
		for k := range j.Args {
			a := &j.Args[k]
			a.Choices = c.argChoices(*a)
			a.Default = a.resolvedDefault()
			if a.Default == "" && len(a.Choices) > 0 {
				a.Default = a.Choices[0]
			}
		}
		out[i] = j
	}
	return out
}

// findJob looks up a catalog entry by group and name.
func findJob(group, name string) (JobSpec, bool) {
	for _, j := range catalog {
		if j.Group == group && j.Name == name {
			return j, true
		}
	}
	return JobSpec{}, false
}

// defaultProject is the .env's PROJECT - what "this repo" means on the host.
func defaultProject() string { return strings.TrimSpace(os.Getenv("PROJECT")) }

// firstNonEmpty is the first non-empty string in ss, or "".
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// safeProjectName rules out path traversal in a single directory name
// under the projects root (a local workspace, a bundle source).
// Deliberately lenient otherwise: real checkouts have underscores, dots,
// mixed case.
func safeProjectName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid directory name %q", name)
	}
	return nil
}

// RunJob validates against the catalog and runs the job in the background.
// One run per job at a time; its id is "<group>/<name>".
func (c *Catalog) RunJob(group, name, project string, args map[string]string) (string, error) {
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
		if choices := c.argChoices(a); choices != nil {
			if v == "" && len(choices) > 0 {
				v = choices[0]
			}
			ok := false
			for _, ch := range choices {
				ok = ok || ch == v
			}
			if !ok {
				return "", fmt.Errorf("%s/%s: %q must be one of %v (deployment.yaml)", group, name, a.Name, choices)
			}
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
	ws := c.mgr.Assignment()
	if spec.Workspace && ws == nil {
		if c.mgr.WorkspaceName() != "" {
			return "", fmt.Errorf("workspace %q hasn't been assigned by the parent gateway yet", c.mgr.WorkspaceName())
		}
		return "", fmt.Errorf("%s/%s is repository work: run it from a workspace's own gateway (root console → Workspaces → start its gateway)", group, name)
	}
	if project == "" || project == "self" {
		project = defaultProject()
	}
	j := &jobRun{c: c, spec: spec, args: resolved, project: project}
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
		if root := c.mgr.ProjectsRoot(); root != "" {
			j.dir = filepath.Join(root, project)
		}
	}
	id := group + "/" + name
	o, err := c.tracker.Start("job:"+id, "starting", "running "+id)
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
		c.tracker.Finish("job:"+id, o, spec.run(ctx, j))
	}()
	return id, nil
}

// JobLog is the log of job id ("<group>/<name>"), current or last run.
func (c *Catalog) JobLog(id string) (string, bool) {
	return c.tracker.Log("job:" + id)
}

// JobRuns reports in-flight/last state per job id, keyed by the "job:"
// op-map namespace. A run that finished cleanly is kept (state "done") so
// its log stays readable.
func (c *Catalog) JobRuns() map[string]map[string]string {
	return c.tracker.Snapshot("job:")
}
