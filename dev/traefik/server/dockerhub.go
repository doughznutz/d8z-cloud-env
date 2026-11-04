package main

// Native Docker Hub jobs: Hub API calls straight from gw-server, plus the
// docker CLI already in this image for build/push/pull. Replaces the old
// deploy/docker container (curl + shell functions run via `make`).

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const hubAPI = "https://hub.docker.com"

func hubCreds() (user, tok string, err error) {
	user = strings.TrimSpace(os.Getenv("DOCKER_HUB_USERNAME"))
	tok = strings.TrimSpace(os.Getenv("DOCKER_HUB_TOKEN"))
	if user == "" || tok == "" {
		return "", "", fmt.Errorf("DOCKER_HUB_USERNAME and DOCKER_HUB_TOKEN must be set in .env")
	}
	return user, tok, nil
}

// hubSession exchanges the username + access token for a Hub API bearer
// token (the current /v2/auth/token, falling back to the older
// /v2/users/login for accounts/tokens that predate it).
type hubSession struct {
	user string
	hdr  http.Header
}

func newHubSession(ctx context.Context) (*hubSession, error) {
	user, tok, err := hubCreds()
	if err != nil {
		return nil, err
	}
	var bearer string
	var a struct {
		AccessToken string `json:"access_token"`
	}
	err = doJSON(ctx, "POST", hubAPI+"/v2/auth/token", nil, map[string]string{"identifier": user, "secret": tok}, &a)
	if err == nil && a.AccessToken != "" {
		bearer = a.AccessToken
	} else {
		var l struct {
			Token string `json:"token"`
		}
		if lerr := doJSON(ctx, "POST", hubAPI+"/v2/users/login", nil, map[string]string{"username": user, "password": tok}, &l); lerr != nil {
			return nil, fmt.Errorf("Docker Hub login as %s: %w", user, lerr)
		}
		bearer = l.Token
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+bearer)
	return &hubSession{user: user, hdr: hdr}, nil
}

func (s *hubSession) do(ctx context.Context, method, path string, body, out any) error {
	return doJSON(ctx, method, hubAPI+path, s.hdr, body, out)
}

var hubRepoRE = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
var hubTagRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

func hubRepo(j *jobRun) (string, error) {
	r := j.arg("repo")
	if r == "" {
		return "", fmt.Errorf("no repository given and DOCKER_HUB_REPO is unset")
	}
	if !hubRepoRE.MatchString(r) {
		return "", fmt.Errorf("invalid Docker Hub repository name %q (lowercase letters, digits, . _ -)", r)
	}
	return r, nil
}

func dhCheckRepo(ctx context.Context, j *jobRun) error {
	repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	s, err := newHubSession(ctx)
	if err != nil {
		return err
	}
	var info struct {
		IsPrivate   bool   `json:"is_private"`
		PullCount   int    `json:"pull_count"`
		LastUpdated string `json:"last_updated"`
	}
	err = s.do(ctx, "GET", "/v2/repositories/"+s.user+"/"+repo+"/", nil, &info)
	if isStatus(err, 404) {
		j.logf("%s/%s does not exist.", s.user, repo)
		return nil
	}
	if err != nil {
		return err
	}
	vis := "public"
	if info.IsPrivate {
		vis = "private"
	}
	j.logf("%s/%s exists: %s, %d pulls, last updated %s\nhttps://hub.docker.com/r/%s/%s", s.user, repo, vis, info.PullCount, info.LastUpdated, s.user, repo)
	return nil
}

func dhCreateRepo(ctx context.Context, j *jobRun) error {
	repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	vis := strings.ToLower(j.arg("visibility"))
	if vis != "public" && vis != "private" {
		return fmt.Errorf("visibility must be public or private, not %q", vis)
	}
	s, err := newHubSession(ctx)
	if err != nil {
		return err
	}
	j.logf("creating %s repository %s/%s ...", vis, s.user, repo)
	body := map[string]any{"namespace": s.user, "name": repo, "is_private": vis == "private", "description": ""}
	if err := s.do(ctx, "POST", "/v2/repositories/", body, nil); err != nil {
		return err
	}
	j.logf("created https://hub.docker.com/r/%s/%s", s.user, repo)
	return nil
}

// hubPages walks a paginated Hub listing, calling each for every result.
func hubPages[T any](ctx context.Context, s *hubSession, path string, each func(T)) error {
	next := hubAPI + path
	for pages := 0; next != "" && pages < 50; pages++ {
		var page struct {
			Next    string `json:"next"`
			Results []T    `json:"results"`
		}
		if err := doJSON(ctx, "GET", next, s.hdr, nil, &page); err != nil {
			return err
		}
		for _, r := range page.Results {
			each(r)
		}
		next = page.Next
	}
	return nil
}

func dhListRepos(ctx context.Context, j *jobRun) error {
	s, err := newHubSession(ctx)
	if err != nil {
		return err
	}
	j.logf("Repositories of %s:", s.user)
	n := 0
	err = hubPages(ctx, s, "/v2/repositories/"+s.user+"/?page_size=100", func(r struct {
		Name        string `json:"name"`
		IsPrivate   bool   `json:"is_private"`
		LastUpdated string `json:"last_updated"`
	}) {
		vis := "public"
		if r.IsPrivate {
			vis = "private"
		}
		j.logf("  %-40s %-8s %s", r.Name, vis, r.LastUpdated)
		n++
	})
	j.logf("%d repositories.", n)
	return err
}

func dhListTags(ctx context.Context, j *jobRun) error {
	repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	s, err := newHubSession(ctx)
	if err != nil {
		return err
	}
	j.logf("Tags of %s/%s:", s.user, repo)
	n := 0
	err = hubPages(ctx, s, "/v2/repositories/"+s.user+"/"+repo+"/tags/?page_size=100", func(t struct {
		Name        string `json:"name"`
		FullSize    int64  `json:"full_size"`
		LastUpdated string `json:"last_updated"`
	}) {
		j.logf("  %-40s %8.1f MB  %s", t.Name, float64(t.FullSize)/1e6, t.LastUpdated)
		n++
	})
	if isStatus(err, 404) {
		return fmt.Errorf("%s/%s does not exist", s.user, repo)
	}
	j.logf("%d tags.", n)
	return err
}

// hubImage is DOCKER_HUB_USERNAME/<repo>:<tag>.
func hubImage(j *jobRun) (string, error) {
	user, _, err := hubCreds()
	if err != nil {
		return "", err
	}
	repo, err := hubRepo(j)
	if err != nil {
		return "", err
	}
	tag := j.arg("tag")
	if !hubTagRE.MatchString(tag) {
		return "", fmt.Errorf("invalid tag %q", tag)
	}
	return user + "/" + repo + ":" + tag, nil
}

// withRegistryLogin runs fn with the docker CLI logged in to Docker Hub
// through a throwaway DOCKER_CONFIG, so the credential never lands in the
// gateway container's own ~/.docker and is gone when the job ends.
func withRegistryLogin(ctx context.Context, j *jobRun, fn func(env []string) error) error {
	user, tok, err := hubCreds()
	if err != nil {
		return err
	}
	cfg, err := os.MkdirTemp("", "d8z-dockercfg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cfg)
	env := []string{"DOCKER_CONFIG=" + cfg}
	j.logf("$ docker login -u %s --password-stdin", user)
	cmd := exec.CommandContext(ctx, "docker", "login", "-u", user, "--password-stdin")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(tok)
	cmd.Stdout, cmd.Stderr = j.log, j.log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker login: %v", err)
	}
	return fn(env)
}

func dhBuildAndPush(ctx context.Context, j *jobRun) error {
	dir, err := j.projectDir()
	if err != nil {
		return err
	}
	image, err := hubImage(j)
	if err != nil {
		return err
	}
	df := filepath.Clean(j.arg("dockerfile"))
	if filepath.IsAbs(df) || strings.HasPrefix(df, "..") {
		return fmt.Errorf("dockerfile %q must be a path inside the project", j.arg("dockerfile"))
	}
	dfPath := filepath.Join(dir, df)
	if _, err := os.Stat(dfPath); err != nil {
		return fmt.Errorf("no dockerfile at %s", dfPath)
	}
	j.logf("image: %s\ncontext: %s\ndockerfile: %s", image, dir, df)
	return withRegistryLogin(ctx, j, func(env []string) error {
		if err := dockerStreamHostEnv(ctx, j.log, "", env, "build", "--pull", "-t", image, "-f", dfPath, dir); err != nil {
			return err
		}
		if err := dockerStreamHostEnv(ctx, j.log, "", env, "push", image); err != nil {
			return err
		}
		repo, tag, _ := strings.Cut(strings.SplitN(image, "/", 2)[1], ":")
		user, _, _ := hubCreds()
		j.logf("\nPushed %s\nhttps://hub.docker.com/r/%s/%s/tags?name=%s", image, user, repo, url.QueryEscape(tag))
		return nil
	})
}

func dhPull(ctx context.Context, j *jobRun) error {
	image, err := hubImage(j)
	if err != nil {
		return err
	}
	return withRegistryLogin(ctx, j, func(env []string) error {
		return dockerStreamHostEnv(ctx, j.log, "", env, "pull", image)
	})
}
