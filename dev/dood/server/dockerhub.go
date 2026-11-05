package main

// Native Docker Hub jobs: Hub API calls straight from gw-server, plus the
// docker CLI already in this image for build/push/pull.
//
// We publish several named containers into ONE Docker Hub repository, one
// tag each: deployment.yaml's docker_hub.images maps a name (the tag) to the
// *-dh service in docker-compose.yaml that builds it, e.g.
//   dood    -> dood-dh    -> doughznutz/doughznutz:dood
//   d8z     -> deightz-dh -> doughznutz/doughznutz:d8z

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

// hubCreds are the Docker Hub account and token from deployment.yaml
// (defaulting to DOCKER_HUB_USERNAME / DOCKER_HUB_TOKEN).
func (m *Manager) hubCreds() (d *Deployment, user, tok string, err error) {
	d, _, err = m.loadDeployment()
	if err != nil {
		return nil, "", "", err
	}
	user, tok = d.DockerHub.Username, d.hubToken()
	if user == "" || tok == "" {
		return nil, "", "", fmt.Errorf("Docker Hub username and token must be set (deployment.yaml docker_hub, or DOCKER_HUB_USERNAME / %s in .env)", d.DockerHub.TokenEnv)
	}
	return d, user, tok, nil
}

// hubSession exchanges the username + access token for a Hub API bearer
// token (the current /v2/auth/token, falling back to the older
// /v2/users/login for accounts/tokens that predate it).
type hubSession struct {
	user string
	hdr  http.Header
}

func (m *Manager) newHubSession(ctx context.Context) (*hubSession, error) {
	_, user, tok, err := m.hubCreds()
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

// hubRepo is the repository all our images are tags of.
func hubRepo(j *jobRun) (string, error) {
	d, _, err := j.m.loadDeployment()
	if err != nil {
		return "", err
	}
	if d.DockerHub.Repository == "" {
		return "", fmt.Errorf("no Docker Hub repository: set docker_hub.repository in deployment.yaml (or DOCKER_HUB_REPO in .env)")
	}
	return d.DockerHub.Repository, nil
}

func dhCheckRepo(ctx context.Context, j *jobRun) error {
	repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	s, err := j.m.newHubSession(ctx)
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
	s, err := j.m.newHubSession(ctx)
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
	s, err := j.m.newHubSession(ctx)
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
	s, err := j.m.newHubSession(ctx)
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

// hubImages resolves a job's image arg ("all" or one name) against
// deployment.yaml.
func hubImages(j *jobRun) (*Deployment, []HubImage, error) {
	d, _, err := j.m.loadDeployment()
	if err != nil {
		return nil, nil, err
	}
	if d.DockerHub.Username == "" || d.DockerHub.Repository == "" {
		return nil, nil, fmt.Errorf("Docker Hub username and repository must be set (deployment.yaml docker_hub)")
	}
	name := j.arg("image")
	if name == "" || name == "all" {
		return d, d.DockerHub.Images, nil
	}
	im, ok := d.image(name)
	if !ok {
		return nil, nil, fmt.Errorf("no image %q in deployment.yaml docker_hub.images", name)
	}
	return d, []HubImage{im}, nil
}

// withRegistryLogin runs fn with the docker CLI logged in to Docker Hub
// through a throwaway DOCKER_CONFIG, so the credential never lands in the
// gateway container's own ~/.docker and is gone when the job ends.
func withRegistryLogin(ctx context.Context, j *jobRun, fn func(env []string) error) error {
	_, user, tok, err := j.m.hubCreds()
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

// dhBuild builds images with `docker compose --profile dh build <service>`
// from the project's live checkout (not the copy baked into this image),
// with the repository names from deployment.yaml so each lands on its
// Docker Hub reference.
func dhBuild(ctx context.Context, j *jobRun, d *Deployment, images []HubImage) error {
	dir, err := j.projectDir()
	if err != nil {
		return err
	}
	compose := filepath.Join(dir, "docker-compose.yaml")
	if _, err := os.Stat(compose); err != nil {
		return fmt.Errorf("no docker-compose.yaml in %s", dir)
	}
	env := []string{"DOCKER_HUB_USERNAME=" + d.DockerHub.Username, "DOCKER_HUB_REPO=" + d.DockerHub.Repository}
	args := []string{"compose", "--project-directory", dir, "-f", compose, "--profile", "dh", "build", "--pull"}
	for _, im := range images {
		j.logf("%s <- service %s", d.ref(im.Name), im.Service)
		args = append(args, im.Service)
	}
	return dockerStreamHostEnv(ctx, j.log, "", env, args...)
}

func dhPush(ctx context.Context, j *jobRun, d *Deployment, images []HubImage) error {
	return withRegistryLogin(ctx, j, func(env []string) error {
		for _, im := range images {
			ref := d.ref(im.Name)
			if err := dockerStreamHostEnv(ctx, j.log, "", env, "push", ref); err != nil {
				return err
			}
			j.logf("pushed %s\nhttps://hub.docker.com/r/%s/%s/tags?name=%s\n", ref,
				d.DockerHub.Username, d.DockerHub.Repository, url.QueryEscape(im.Name))
		}
		return nil
	})
}

func dhBuildJob(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	return dhBuild(ctx, j, d, images)
}

func dhPushJob(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	return dhPush(ctx, j, d, images)
}

func dhBuildAndPush(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	if err := dhBuild(ctx, j, d, images); err != nil {
		return err
	}
	return dhPush(ctx, j, d, images)
}

func dhPull(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	for _, im := range images {
		if err := dockerStream(ctx, j.log, "pull", d.ref(im.Name)); err != nil {
			return err
		}
	}
	return nil
}
