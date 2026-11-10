package jobs

// Native GitHub jobs: REST calls straight from gw-server, plus the git CLI
// already in this image for anything that moves files. Replaces the old
// deploy/github container (curl + shell functions).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"d8z/gw/internal/apiclient"
)

// githubAPI is the GitHub REST API base URL.
const githubAPI = "https://api.github.com"

// editorUID and editorGID own workspace files: Deightz's PUID/PGID (the
// same pattern main's workspace mounts use).
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

// ghRepo is a GitHub repository, owner/name.
type ghRepo struct{ Owner, Name string }

// String is the repo as "owner/name".
func (r ghRepo) String() string { return r.Owner + "/" + r.Name }

// wsTokenKey carries a workspace's own GitHub token through a job's ctx.
// Present (even empty) means "this job acts for a workspace": its token is
// the only one used - never a silent fallback to the .env's GITHUB_TOKEN.
type wsTokenKey struct{}

// wsTokenVal is the value stored under wsTokenKey: the workspace's name and
// its token.
type wsTokenVal struct{ ws, tok string }

// withWorkspaceToken marks ctx as acting for workspace ws, with its token.
func withWorkspaceToken(ctx context.Context, ws, tok string) context.Context {
	return context.WithValue(ctx, wsTokenKey{}, wsTokenVal{ws, tok})
}

// githubToken is the token a GitHub call uses: the workspace's own when ctx
// carries one (an error if it's empty), else GITHUB_TOKEN from the env.
func githubToken(ctx context.Context) (string, error) {
	if v, ok := ctx.Value(wsTokenKey{}).(wsTokenVal); ok {
		if v.tok == "" {
			return "", fmt.Errorf("workspace %q has no GitHub token - set one on the root console's Workspaces tab", v.ws)
		}
		return v.tok, nil
	}
	t := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if t == "" {
		return "", fmt.Errorf("GITHUB_TOKEN is not set in .env (or the shell compose ran from)")
	}
	return t, nil
}

// gh makes an authenticated JSON call to the GitHub REST API at path.
func gh(ctx context.Context, method, path string, body, out any) error {
	tok, err := githubToken(ctx)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+tok)
	hdr.Set("Accept", "application/vnd.github+json")
	hdr.Set("X-GitHub-Api-Version", "2022-11-28")
	return apiclient.DoJSON(ctx, method, githubAPI+path, hdr, body, out)
}

// ghLogin is the token's own account - the authority on who we are,
// rather than trusting GITHUB_USER to match the token.
func ghLogin(ctx context.Context) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := gh(ctx, "GET", "/user", nil, &u); err != nil {
		return "", fmt.Errorf("GitHub token check: %w", err)
	}
	return u.Login, nil
}

// Patterns for GitHub owner/repo names and github.com remote URLs (HTTPS or
// SSH), the latter capturing owner and name.
var (
	ghNameRE   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	ghOriginRE = regexp.MustCompile(`github\.com[:/]+([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+?)(?:\.git)?/?$`)
)

// parseGHRepo reads a repo from "owner/name", a github.com URL, or a bare
// name (owner from GITHUB_ORG, else GITHUB_USER).
func parseGHRepo(s string) (ghRepo, error) {
	s = strings.TrimSpace(s)
	if m := ghOriginRE.FindStringSubmatch(s); m != nil {
		return ghRepo{m[1], m[2]}, nil
	}
	owner, name, ok := strings.Cut(s, "/")
	if !ok {
		owner, name = strings.TrimSpace(os.Getenv("GITHUB_ORG")), s
		if owner == "" {
			owner = strings.TrimSpace(os.Getenv("GITHUB_USER"))
		}
		if owner == "" {
			return ghRepo{}, fmt.Errorf("repo %q has no owner and GITHUB_ORG/GITHUB_USER are unset", s)
		}
	}
	if !ghNameRE.MatchString(owner) || !ghNameRE.MatchString(name) {
		return ghRepo{}, fmt.Errorf("invalid repo %q (want owner/name)", s)
	}
	return ghRepo{owner, name}, nil
}

// ParseGHRepo normalizes a GitHub repo reference - already "owner/name",
// or a github.com clone URL (HTTPS or SSH) - to "owner/name". Exported for
// main's own workspace validation, which accepts a pasted
// clone URL the same way a job's repo arg does.
func ParseGHRepo(s string) (string, error) {
	r, err := parseGHRepo(s)
	if err != nil {
		return "", err
	}
	return r.String(), nil
}

// originURL reads remote.origin.url straight out of <dir>/.git/config - no
// git process, so no safe.directory complaints about who owns the checkout.
func originURL(dir string) string {
	f, err := os.Open(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			in = strings.ReplaceAll(line, " ", "") == `[remote"origin"]`
			continue
		}
		if k, v, ok := strings.Cut(line, "="); in && ok && strings.TrimSpace(k) == "url" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ghTarget resolves which repo a job acts on: the explicit repo arg, else
// the project's own github.com origin, else GITHUB_ORG/GITHUB_REPO - the
// latter only for the .env's own PROJECT, so a different project never
// silently lands in this repo's GitHub.
func ghTarget(j *jobRun) (ghRepo, error) {
	if s := j.arg("repo"); s != "" {
		return parseGHRepo(s)
	}
	if j.dir != "" {
		if o := originURL(j.dir); o != "" {
			if r, err := parseGHRepo(o); err == nil && strings.Contains(o, "github.com") {
				j.logf("repo: %s (from the project's git origin)", r)
				return r, nil
			}
		}
	}
	if j.project == defaultProject() {
		if name := strings.TrimSpace(os.Getenv("GITHUB_REPO")); name != "" {
			r, err := parseGHRepo(name)
			if err == nil {
				j.logf("repo: %s (from .env GITHUB_REPO)", r)
			}
			return r, err
		}
	}
	return ghRepo{}, fmt.Errorf("no repo for project %q: it has no github.com origin - give a repo (owner/name)", j.project)
}

// ghRepoInfo is the subset of GitHub's repository object we read.
type ghRepoInfo struct {
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	PushedAt      string `json:"pushed_at"`
}

// ghCheckRepo is the github/check_repo job: logs whether the repo exists
// (and the token can see it), its visibility and default branch.
func ghCheckRepo(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	var info ghRepoInfo
	err = gh(ctx, "GET", "/repos/"+r.String(), nil, &info)
	if apiclient.IsStatus(err, 404) {
		j.logf("%s does not exist (or the token can't see it).", r)
		return nil
	}
	if err != nil {
		return err
	}
	vis := "public"
	if info.Private {
		vis = "private"
	}
	j.logf("%s exists: %s, default branch %q, last push %s\n%s", info.FullName, vis, info.DefaultBranch, info.PushedAt, info.HTMLURL)
	return nil
}

// ghCreateRepo is the github/create_repo job: creates the repo with an
// initial README, under the token's own account or an organization.
func ghCreateRepo(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	vis := strings.ToLower(j.arg("visibility"))
	if vis != "public" && vis != "private" {
		return fmt.Errorf("visibility must be public or private, not %q", vis)
	}
	login, err := ghLogin(ctx)
	if err != nil {
		return err
	}
	path := "/orgs/" + url.PathEscape(r.Owner) + "/repos"
	if strings.EqualFold(r.Owner, login) {
		path = "/user/repos"
	}
	j.logf("creating %s %s ...", vis, r)
	var info ghRepoInfo
	body := map[string]any{"name": r.Name, "private": vis == "private", "auto_init": true}
	if err := gh(ctx, "POST", path, body, &info); err != nil {
		return err
	}
	j.logf("created %s (default branch %q)\n%s", info.FullName, info.DefaultBranch, info.HTMLURL)
	return nil
}

// --- git plumbing -----------------------------------------------------------

// GitEnv authenticates github.com over HTTPS with the token as an extra
// header, passed through GIT_CONFIG_* so it never appears in argv, a remote
// URL, or the job log; and lets root (gw-server) read checkouts owned by the
// host user.
// An empty token (a public repo) sends no header at all.
//
// Exported: main's own workspace-status check runs read-only
// git commands (status, rev-list) the same way, with an empty token.
func GitEnv(tok string) []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*")
	if tok != "" {
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + tok))
		env = append(env, "GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_1=http.https://github.com/.extraheader",
			"GIT_CONFIG_VALUE_1=Authorization: Basic "+basic)
	}
	return env
}

// GitCmd runs git with a fixed context, log, environment and leading
// arguments. Exported for the same reason as GitEnv.
type GitCmd struct {
	Ctx    context.Context
	Log    io.Writer // where Run echoes the command and its output
	Env    []string  // process environment (see GitEnv)
	Pre    []string  // --git-dir / --work-tree, or -C <dir>
	Editor bool      // run as the editor user, for anything that writes into a workspace
}

// Command builds the git process for args, without starting it.
func (g GitCmd) Command(args []string) *exec.Cmd {
	cmd := exec.CommandContext(g.Ctx, "git", append(append([]string{}, g.Pre...), args...)...)
	cmd.Env = g.Env
	if g.Editor {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: editorUID, Gid: editorGID}}
		cmd.Env = append(append([]string{}, g.Env...), "HOME=/tmp")
	}
	return cmd
}

// Run runs git args with the command and its output written to g.Log.
func (g GitCmd) Run(args ...string) error {
	fmt.Fprintf(g.Log, "$ git %s\n", strings.Join(args, " "))
	cmd := g.Command(args)
	cmd.Stdout, cmd.Stderr = g.Log, g.Log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %v", args[0], err)
	}
	return nil
}

// Out runs git args and returns its combined, trimmed output.
func (g GitCmd) Out(args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := g.Command(args)
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// projectExcludes keep secrets and local tooling state out of anything we
// push, on top of the project's own .gitignore files: every .env, and every
// top-level dotfile except the few that belong in a repo.
const projectExcludes = `.env
.env.*
!.env.example
!.env.sample
!.env.template
/.*
!/.gitignore
!/.gitattributes
!/.dockerignore
!/.github
`

// ghSnapshot stages the project's current tree against a shallow clone of
// the repo's default branch, without copying anything: the clone's .git is
// used with the project itself as its work tree, so git applies the
// project's .gitignore files, picks up deletions, and never writes into
// the project. Returns the git runner (scoped to the snapshot) and a
// cleanup func.
func ghSnapshot(ctx context.Context, j *jobRun, r ghRepo) (GitCmd, func(), error) {
	dir, err := j.projectDir()
	if err != nil {
		return GitCmd{}, nil, err
	}
	tok, err := githubToken(ctx)
	if err != nil {
		return GitCmd{}, nil, err
	}
	tmp, err := os.MkdirTemp("", "d8z-gh-")
	if err != nil {
		return GitCmd{}, nil, err
	}
	cleanup := func() { os.RemoveAll(tmp) }
	clone := filepath.Join(tmp, "repo")
	g := GitCmd{Ctx: ctx, Log: j.log, Env: GitEnv(tok)}
	if err := g.Run("clone", "--quiet", "--depth", "1", "--no-checkout", "https://github.com/"+r.String()+".git", clone); err != nil {
		cleanup()
		return GitCmd{}, nil, err
	}
	gitDir := filepath.Join(clone, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte(projectExcludes), 0o644); err != nil {
		cleanup()
		return GitCmd{}, nil, err
	}
	g.Pre = []string{"--git-dir", gitDir, "--work-tree", dir}
	// Index := the default branch's tree (an empty repo has no HEAD yet).
	if _, err := g.Out("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		if err := g.Run("read-tree", "HEAD"); err != nil {
			cleanup()
			return GitCmd{}, nil, err
		}
	}
	if err := g.Run("add", "--all", "."); err != nil {
		cleanup()
		return GitCmd{}, nil, err
	}
	return g, cleanup, nil
}

// ghDiffRepo is the github/diff_repo job: logs what push_branch would
// commit, against the repo's default branch.
func ghDiffRepo(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	g, cleanup, err := ghSnapshot(ctx, j, r)
	if err != nil {
		return err
	}
	defer cleanup()
	stat, err := g.Out("diff", "--cached", "--stat=120")
	if err != nil {
		return fmt.Errorf("git diff: %v: %s", err, stat)
	}
	if stat == "" {
		j.logf("\nNo differences: the project matches %s's default branch.", r)
		return nil
	}
	names, _ := g.Out("diff", "--cached", "--name-status")
	j.logf("\nChanges a push_branch would commit (A added, M modified, D deleted):\n%s\n\n%s", names, stat)
	return nil
}

// branchRE is the characters a branch name we accept may use.
var branchRE = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

// ghPushBranch is the github/push_branch job: commits the project's current
// tree and pushes it as branch (default auto-update-<unix time>).
func ghPushBranch(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	branch := j.arg("branch")
	if branch == "" {
		branch = fmt.Sprintf("auto-update-%d", time.Now().Unix())
	}
	if !branchRE.MatchString(branch) || strings.Contains(branch, "..") || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	login, err := ghLogin(ctx)
	if err != nil {
		return err
	}
	// A GitHub workspace's checkout is ours (it lives in its volume): commit
	// right there, so the workspace's own history - and the open-work check
	// before a stop - reflect what was pushed. A local workspace is the
	// user's own directory, so it's pushed from a snapshot and never written.
	if a := j.c.mgr.Assignment(); a != nil && a.Source == "github" {
		if _, isGit, _ := wsGitState(j.dir); isGit {
			return ghPushInPlace(ctx, j, r, branch, login)
		}
	}
	g, cleanup, err := ghSnapshot(ctx, j, r)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := g.Out("diff", "--cached", "--quiet"); err == nil {
		if _, herr := g.Out("rev-parse", "--verify", "-q", "HEAD"); herr == nil {
			j.logf("\nNothing to push: the project matches %s's default branch.", r)
			return nil
		}
	}
	stat, _ := g.Out("diff", "--cached", "--stat=120")
	j.logf("%s", stat)
	if err := g.Run("-c", "user.name="+login, "-c", "user.email="+login+"@users.noreply.github.com",
		"commit", "--quiet", "-m", "Automated update from d8z gateway ("+j.project+")"); err != nil {
		return err
	}
	if err := g.Run("push", "--quiet", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return err
	}
	j.logf("\nPushed branch %q to %s.\nOpen a PR with create_pull_request (branch %s), or: https://github.com/%s/compare/%s?expand=1",
		branch, r, branch, r, url.PathEscape(branch))
	return nil
}

// ghCreatePR is the github/create_pull_request job: opens a PR from branch
// into the repo's default branch.
func ghCreatePR(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	var info ghRepoInfo
	if err := gh(ctx, "GET", "/repos/"+r.String(), nil, &info); err != nil {
		return err
	}
	branch := j.arg("branch")
	title := j.arg("title")
	if title == "" {
		title = "Automated update - " + branch
	}
	var pr struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	err = gh(ctx, "POST", "/repos/"+r.String()+"/pulls", map[string]any{
		"title": title, "head": branch, "base": info.DefaultBranch,
		"body":                  "Opened from the d8z gateway console (project `" + j.project + "`).",
		"maintainer_can_modify": true,
	}, &pr)
	if err != nil {
		return err
	}
	j.logf("Opened PR #%d: %s -> %s\n%s", pr.Number, branch, info.DefaultBranch, pr.HTMLURL)
	return nil
}

// ghRenameBranch is the github/rename_branch job.
func ghRenameBranch(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	oldName, newName := j.arg("old_name"), j.arg("new_name")
	if !branchRE.MatchString(newName) || strings.Contains(newName, "..") {
		return fmt.Errorf("invalid branch name %q", newName)
	}
	var out struct {
		Name string `json:"name"`
	}
	path := "/repos/" + r.String() + "/branches/" + url.PathEscape(oldName) + "/rename"
	if err := gh(ctx, "POST", path, map[string]string{"new_name": newName}, &out); err != nil {
		return err
	}
	j.logf("Renamed %s: %q -> %q", r, oldName, out.Name)
	return nil
}

// ghPull brings the workspace up to date with GitHub: a fresh clone into an
// empty workspace (a new github workspace's volume), else a fast-forward-only
// pull of the checkout that's there. Runs as the editor user, so the files
// stay Deightz's. Only ever runs in the workspace's own gateway - the one
// container (besides its Deightz) that mounts it read/write.
func ghPull(ctx context.Context, j *jobRun) error {
	dir, err := j.projectDir()
	if err != nil {
		return err
	}
	// Public repos pull without a token.
	tok, _ := githubToken(ctx)
	g := GitCmd{Ctx: ctx, Log: j.log, Env: GitEnv(tok), Editor: true}
	if present, isGit, head := wsGitState(dir); isGit {
		j.logf("checkout on %s - fast-forwarding", head)
		g.Pre = []string{"-C", dir}
		return g.Run("pull", "--ff-only", "--no-rebase")
	} else if present {
		return fmt.Errorf("%s isn't a git checkout and isn't empty - nothing to pull into", dir)
	}
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	args := []string{"clone"}
	if b := j.arg("branch"); b != "" {
		args = append(args, "--branch", b)
	}
	if err := g.Run(append(args, "https://github.com/"+r.String()+".git", dir)...); err != nil {
		return err
	}
	ensureExcludes(dir)
	_, _, head := wsGitState(dir)
	j.logf("\ncloned %s (%s) into %s", r, head, dir)
	return nil
}

// ghPushInPlace commits everything in a GitHub workspace's checkout (minus
// projectExcludes) and pushes HEAD as `branch`.
func ghPushInPlace(ctx context.Context, j *jobRun, r ghRepo, branch, login string) error {
	tok, err := githubToken(ctx)
	if err != nil {
		return err
	}
	ensureExcludes(j.dir)
	g := GitCmd{Ctx: ctx, Log: j.log, Env: GitEnv(tok), Pre: []string{"-C", j.dir}, Editor: true}
	if err := g.Run("add", "--all", "."); err != nil {
		return err
	}
	if _, err := g.Out("diff", "--cached", "--quiet"); err != nil {
		stat, _ := g.Out("diff", "--cached", "--stat=120")
		j.logf("%s", stat)
		if err := g.Run("-c", "user.name="+login, "-c", "user.email="+login+"@users.noreply.github.com",
			"commit", "--quiet", "-m", "Update from d8z workspace "+j.project); err != nil {
			return err
		}
	} else if n, _ := g.Out("rev-list", "--count", "HEAD", "--not", "--remotes"); n == "0" {
		j.logf("\nNothing to push: no uncommitted changes and every commit is already on GitHub.")
		return nil
	}
	if err := g.Run("push", "--quiet", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return err
	}
	j.logf("\nCommitted in the workspace and pushed branch %q to %s.\nOpen a PR with create_pull_request (branch %s), or: https://github.com/%s/compare/%s?expand=1",
		branch, r, branch, r, url.PathEscape(branch))
	return nil
}

// ensureExcludes adds projectExcludes to a checkout's .git/info/exclude, so
// a workspace commit never picks up .env files or tooling dotfiles.
func ensureExcludes(dir string) {
	f := filepath.Join(dir, ".git", "info", "exclude")
	old, _ := os.ReadFile(f)
	if strings.Contains(string(old), "# d8z excludes") {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	os.WriteFile(f, append(old, []byte("\n# d8z excludes\n"+projectExcludes)...), 0o644)
	os.Chown(f, editorUID, editorGID)
}
