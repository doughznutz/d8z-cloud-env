package jobs

// Native Docker Hub jobs: Hub API calls straight from gw-server, plus the
// docker CLI already in this image for build/push/pull.
//
// We publish several named containers into ONE Docker Hub repository, one
// tag each: deployment.yaml's docker_hub.images maps a name (the tag) to the
// *-dh service in docker-compose.yaml that builds it, e.g.
//   dood    -> dood-dh    -> doughznutz/doughznutz:dood
//   d8z     -> deightz-dh -> doughznutz/doughznutz:d8z
//
// Docker Hub credentials, sessions and validation patterns live in the
// deployment package (deployment.yaml's own domain); this file is only the
// job bodies that act on them.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"d8z/gw/deployment"
	"d8z/gw/internal/apiclient"
)

// hubRepo is the repository all our images are tags of.
func hubRepo(j *jobRun) (*deployment.Deployment, string, error) {
	d, _, err := j.c.deploy.Load()
	if err != nil {
		return nil, "", err
	}
	if d.DockerHub.Repository == "" {
		return nil, "", fmt.Errorf("no Docker Hub repository: set docker_hub.repository in deployment.yaml (or DOCKER_HUB_REPO in .env)")
	}
	return d, d.DockerHub.Repository, nil
}

// dhCheckRepo is the docker/check_repo job: logs whether the repository
// exists, and if so its visibility, pulls and last update.
func dhCheckRepo(ctx context.Context, j *jobRun) error {
	d, repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	s, err := deployment.NewHubSession(ctx, d)
	if err != nil {
		return err
	}
	var info struct {
		IsPrivate   bool   `json:"is_private"`
		PullCount   int    `json:"pull_count"`
		LastUpdated string `json:"last_updated"`
	}
	err = s.Do(ctx, "GET", "/v2/repositories/"+s.User+"/"+repo+"/", nil, &info)
	if apiclient.IsStatus(err, 404) {
		j.logf("%s/%s does not exist.", s.User, repo)
		return nil
	}
	if err != nil {
		return err
	}
	vis := "public"
	if info.IsPrivate {
		vis = "private"
	}
	j.logf("%s/%s exists: %s, %d pulls, last updated %s\nhttps://hub.docker.com/r/%s/%s", s.User, repo, vis, info.PullCount, info.LastUpdated, s.User, repo)
	return nil
}

// dhCreateRepo is the docker/create_repo job: creates the repository with
// the requested visibility.
func dhCreateRepo(ctx context.Context, j *jobRun) error {
	d, repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	vis := strings.ToLower(j.arg("visibility"))
	if vis != "public" && vis != "private" {
		return fmt.Errorf("visibility must be public or private, not %q", vis)
	}
	s, err := deployment.NewHubSession(ctx, d)
	if err != nil {
		return err
	}
	j.logf("creating %s repository %s/%s ...", vis, s.User, repo)
	body := map[string]any{"namespace": s.User, "name": repo, "is_private": vis == "private", "description": ""}
	if err := s.Do(ctx, "POST", "/v2/repositories/", body, nil); err != nil {
		return err
	}
	j.logf("created https://hub.docker.com/r/%s/%s", s.User, repo)
	return nil
}

// dhListRepos is the docker/list_repos job: logs every repository of the
// account.
func dhListRepos(ctx context.Context, j *jobRun) error {
	d, _, err := j.c.deploy.Load()
	if err != nil {
		return err
	}
	s, err := deployment.NewHubSession(ctx, d)
	if err != nil {
		return err
	}
	j.logf("Repositories of %s:", s.User)
	n := 0
	err = deployment.HubPages(ctx, s, "/v2/repositories/"+s.User+"/?page_size=100", func(r struct {
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

// dhListTags is the docker/list_tags job: logs every tag of the repository
// with its size and last update.
func dhListTags(ctx context.Context, j *jobRun) error {
	d, repo, err := hubRepo(j)
	if err != nil {
		return err
	}
	s, err := deployment.NewHubSession(ctx, d)
	if err != nil {
		return err
	}
	j.logf("Tags of %s/%s:", s.User, repo)
	n := 0
	err = deployment.HubPages(ctx, s, "/v2/repositories/"+s.User+"/"+repo+"/tags/?page_size=100", func(t struct {
		Name        string `json:"name"`
		FullSize    int64  `json:"full_size"`
		LastUpdated string `json:"last_updated"`
	}) {
		j.logf("  %-40s %8.1f MB  %s", t.Name, float64(t.FullSize)/1e6, t.LastUpdated)
		n++
	})
	if apiclient.IsStatus(err, 404) {
		return fmt.Errorf("%s/%s does not exist", s.User, repo)
	}
	j.logf("%d tags.", n)
	return err
}

// hubImages resolves a job's image arg ("all" or one name) against
// deployment.yaml.
func hubImages(j *jobRun) (*deployment.Deployment, []deployment.HubImage, error) {
	d, _, err := j.c.deploy.Load()
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
	im, ok := d.Image(name)
	if !ok {
		return nil, nil, fmt.Errorf("no image %q in deployment.yaml docker_hub.images", name)
	}
	return d, []deployment.HubImage{im}, nil
}

// withRegistryLogin runs fn with the docker CLI logged in to Docker Hub
// through a throwaway DOCKER_CONFIG, so the credential never lands in the
// gateway container's own ~/.docker and is gone when the job ends.
func withRegistryLogin(ctx context.Context, j *jobRun, d *deployment.Deployment, fn func(env []string) error) error {
	user, tok, err := deployment.HubCreds(d)
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
func dhBuild(ctx context.Context, j *jobRun, d *deployment.Deployment, images []deployment.HubImage) error {
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
		j.logf("%s <- service %s", d.Ref(im.Name), im.Service)
		args = append(args, im.Service)
	}
	return j.c.mgr.DockerStreamEnv(ctx, j.log, env, args...)
}

// dhPush pushes the already-built images to their Docker Hub references.
func dhPush(ctx context.Context, j *jobRun, d *deployment.Deployment, images []deployment.HubImage) error {
	return withRegistryLogin(ctx, j, d, func(env []string) error {
		for _, im := range images {
			ref := d.Ref(im.Name)
			if err := j.c.mgr.DockerStreamEnv(ctx, j.log, env, "push", ref); err != nil {
				return err
			}
			j.logf("pushed %s\nhttps://hub.docker.com/r/%s/%s/tags?name=%s\n", ref,
				d.DockerHub.Username, d.DockerHub.Repository, url.QueryEscape(im.Name))
		}
		return nil
	})
}

// dhBuildJob is the docker/build job.
func dhBuildJob(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	return dhBuild(ctx, j, d, images)
}

// dhPushJob is the docker/push job.
func dhPushJob(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	return dhPush(ctx, j, d, images)
}

// dhBuildAndPush is the docker/build_and_push job: build, then push only if
// every build succeeded.
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

// dhPull is the docker/pull job: pulls the images to this daemon.
func dhPull(ctx context.Context, j *jobRun) error {
	d, images, err := hubImages(j)
	if err != nil {
		return err
	}
	for _, im := range images {
		if err := j.c.mgr.DockerStream(ctx, j.log, "pull", d.Ref(im.Name)); err != nil {
			return err
		}
	}
	return nil
}
