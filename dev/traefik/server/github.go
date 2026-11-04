package main

// Native GitHub jobs: REST calls straight from gw-server, plus the git CLI
// already in this image for anything that moves files. Replaces the old
// deploy/github container (curl + shell functions run via `make`).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const githubAPI = "https://api.github.com"

var httpClient = &http.Client{Timeout: 60 * time.Second}

type ghRepo struct{ Owner, Name string }

func (r ghRepo) String() string { return r.Owner + "/" + r.Name }

// apiError is a non-2xx answer from GitHub or Docker Hub.
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	msg := strings.TrimSpace(e.Body)
	var v struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Errors  []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal([]byte(e.Body), &v) == nil {
		parts := []string{}
		for _, s := range []string{v.Message, v.Detail} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		for _, er := range v.Errors {
			if er.Message != "" {
				parts = append(parts, er.Message)
			} else if er.Code != "" {
				parts = append(parts, er.Field+" "+er.Code)
			}
		}
		if len(parts) > 0 {
			msg = strings.Join(parts, "; ")
		}
	}
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, msg)
}

func isStatus(err error, code int) bool {
	ae, ok := err.(*apiError)
	return ok && ae.Status == code
}

// doJSON sends body (if non-nil) as JSON and decodes a 2xx response into
// out (if non-nil).
func doJSON(ctx context.Context, method, u string, hdr http.Header, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "d8z-gw-server")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &apiError{Status: resp.StatusCode, Body: string(b)}
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

func githubToken() (string, error) {
	t := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if t == "" {
		return "", fmt.Errorf("GITHUB_TOKEN is not set in .env (or the shell compose ran from)")
	}
	return t, nil
}

func gh(ctx context.Context, method, path string, body, out any) error {
	tok, err := githubToken()
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+tok)
	hdr.Set("Accept", "application/vnd.github+json")
	hdr.Set("X-GitHub-Api-Version", "2022-11-28")
	return doJSON(ctx, method, githubAPI+path, hdr, body, out)
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

var (
	ghNameRE   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	ghOriginRE = regexp.MustCompile(`github\.com[:/]+([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+?)(?:\.git)?/?$`)
)

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

type ghRepoInfo struct {
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	PushedAt      string `json:"pushed_at"`
}

func ghCheckRepo(ctx context.Context, j *jobRun) error {
	r, err := ghTarget(j)
	if err != nil {
		return err
	}
	var info ghRepoInfo
	err = gh(ctx, "GET", "/repos/"+r.String(), nil, &info)
	if isStatus(err, 404) {
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

// gitEnv authenticates github.com over HTTPS with the token as an extra
// header, passed through GIT_CONFIG_* so it never appears in argv, a remote
// URL, or the job log; and lets root (gw-server) read checkouts owned by the
// host user.
func gitEnv(tok string) []string {
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + tok))
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic "+basic,
		"GIT_CONFIG_KEY_1=safe.directory",
		"GIT_CONFIG_VALUE_1=*",
	)
}

type gitCmd struct {
	ctx context.Context
	log io.Writer
	env []string
	pre []string // --git-dir / --work-tree
}

func (g gitCmd) run(args ...string) error {
	all := append(append([]string{}, g.pre...), args...)
	fmt.Fprintf(g.log, "$ git %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(g.ctx, "git", all...)
	cmd.Env, cmd.Stdout, cmd.Stderr = g.env, g.log, g.log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %v", args[0], err)
	}
	return nil
}

func (g gitCmd) out(args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(g.ctx, "git", append(append([]string{}, g.pre...), args...)...)
	cmd.Env, cmd.Stdout, cmd.Stderr = g.env, &buf, &buf
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
func ghSnapshot(ctx context.Context, j *jobRun, r ghRepo) (gitCmd, func(), error) {
	dir, err := j.projectDir()
	if err != nil {
		return gitCmd{}, nil, err
	}
	tok, err := githubToken()
	if err != nil {
		return gitCmd{}, nil, err
	}
	tmp, err := os.MkdirTemp("", "d8z-gh-")
	if err != nil {
		return gitCmd{}, nil, err
	}
	cleanup := func() { os.RemoveAll(tmp) }
	clone := filepath.Join(tmp, "repo")
	g := gitCmd{ctx: ctx, log: j.log, env: gitEnv(tok)}
	if err := g.run("clone", "--quiet", "--depth", "1", "--no-checkout", "https://github.com/"+r.String()+".git", clone); err != nil {
		cleanup()
		return gitCmd{}, nil, err
	}
	gitDir := filepath.Join(clone, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte(projectExcludes), 0o644); err != nil {
		cleanup()
		return gitCmd{}, nil, err
	}
	g.pre = []string{"--git-dir", gitDir, "--work-tree", dir}
	// Index := the default branch's tree (an empty repo has no HEAD yet).
	if _, err := g.out("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		if err := g.run("read-tree", "HEAD"); err != nil {
			cleanup()
			return gitCmd{}, nil, err
		}
	}
	if err := g.run("add", "--all", "."); err != nil {
		cleanup()
		return gitCmd{}, nil, err
	}
	return g, cleanup, nil
}

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
	stat, err := g.out("diff", "--cached", "--stat=120")
	if err != nil {
		return fmt.Errorf("git diff: %v: %s", err, stat)
	}
	if stat == "" {
		j.logf("\nNo differences: the project matches %s's default branch.", r)
		return nil
	}
	names, _ := g.out("diff", "--cached", "--name-status")
	j.logf("\nChanges a push_branch would commit (A added, M modified, D deleted):\n%s\n\n%s", names, stat)
	return nil
}

var branchRE = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

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
	g, cleanup, err := ghSnapshot(ctx, j, r)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := g.out("diff", "--cached", "--quiet"); err == nil {
		if _, herr := g.out("rev-parse", "--verify", "-q", "HEAD"); herr == nil {
			j.logf("\nNothing to push: the project matches %s's default branch.", r)
			return nil
		}
	}
	stat, _ := g.out("diff", "--cached", "--stat=120")
	j.logf("%s", stat)
	if err := g.run("-c", "user.name="+login, "-c", "user.email="+login+"@users.noreply.github.com",
		"commit", "--quiet", "-m", "Automated update from d8z gateway ("+j.project+")"); err != nil {
		return err
	}
	if err := g.run("push", "--quiet", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return err
	}
	j.logf("\nPushed branch %q to %s.\nOpen a PR with create_pull_request (branch %s), or: https://github.com/%s/compare/%s?expand=1",
		branch, r, branch, r, url.PathEscape(branch))
	return nil
}

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
