package deployment

// Docker Hub account/session handling: deployment.yaml's docker_hub
// credentials, and the Hub API bearer-token session they log into. Shared
// by this package's own /api/deployment/docker-hub/repo handler and by the
// jobs package's Docker Hub job bodies (build/push/check/create/list) -
// jobs depends on deployment, never the other way, so this lives here.

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"d8z/gw/internal/apiclient"
)

// hubAPI is the Docker Hub API base URL.
const hubAPI = "https://hub.docker.com"

// hubRepoRE is a valid Docker Hub namespace or repository name.
var hubRepoRE = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

// hubTagRE is a valid image tag.
var hubTagRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// HubCreds are the Docker Hub account and token from d (already loaded,
// with defaults applied).
func HubCreds(d *Deployment) (user, tok string, err error) {
	user, tok = d.DockerHub.Username, d.hubToken()
	if user == "" || tok == "" {
		return "", "", fmt.Errorf("Docker Hub username and token must be set (deployment.yaml docker_hub, or DOCKER_HUB_USERNAME / %s in .env)", d.DockerHub.TokenEnv)
	}
	return user, tok, nil
}

// HubSession is a logged-in Docker Hub API client: the account name and
// the Authorization header carrying its bearer token.
type HubSession struct {
	User string
	hdr  http.Header
}

// NewHubSession exchanges d's username + access token for a Hub API
// bearer token (the current /v2/auth/token, falling back to the older
// /v2/users/login for accounts/tokens that predate it).
func NewHubSession(ctx context.Context, d *Deployment) (*HubSession, error) {
	user, tok, err := HubCreds(d)
	if err != nil {
		return nil, err
	}
	var bearer string
	var a struct {
		AccessToken string `json:"access_token"`
	}
	err = apiclient.DoJSON(ctx, "POST", hubAPI+"/v2/auth/token", nil, map[string]string{"identifier": user, "secret": tok}, &a)
	if err == nil && a.AccessToken != "" {
		bearer = a.AccessToken
	} else {
		var l struct {
			Token string `json:"token"`
		}
		if lerr := apiclient.DoJSON(ctx, "POST", hubAPI+"/v2/users/login", nil, map[string]string{"username": user, "password": tok}, &l); lerr != nil {
			return nil, fmt.Errorf("Docker Hub login as %s: %w", user, lerr)
		}
		bearer = l.Token
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+bearer)
	return &HubSession{User: user, hdr: hdr}, nil
}

// Do makes an authenticated JSON call to the Hub API at path.
func (s *HubSession) Do(ctx context.Context, method, path string, body, out any) error {
	return apiclient.DoJSON(ctx, method, hubAPI+path, s.hdr, body, out)
}

// HubPages walks a paginated Hub listing, calling each for every result.
func HubPages[T any](ctx context.Context, s *HubSession, path string, each func(T)) error {
	next := hubAPI + path
	for pages := 0; next != "" && pages < 50; pages++ {
		var page struct {
			Next    string `json:"next"`
			Results []T    `json:"results"`
		}
		if err := apiclient.DoJSON(ctx, "GET", next, s.hdr, nil, &page); err != nil {
			return err
		}
		for _, r := range page.Results {
			each(r)
		}
		next = page.Next
	}
	return nil
}

// HubRepoInfo is api.hubRepoStatus()'s result: whether the configured
// Docker Hub repository exists, and if so its visibility and stats.
type HubRepoInfo struct {
	Exists  bool   `json:"exists"`
	Private bool   `json:"private,omitempty"`
	Pulls   int    `json:"pulls,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// HubRepoStatus asks Docker Hub about the configured repository.
func (s *Store) HubRepoStatus(ctx context.Context) (*HubRepoInfo, error) {
	d, _, err := s.Load()
	if err != nil {
		return nil, err
	}
	h := d.DockerHub
	if h.Username == "" || h.Repository == "" {
		return nil, fmt.Errorf("set the Docker Hub username and repository first")
	}
	var info struct {
		IsPrivate   bool   `json:"is_private"`
		PullCount   int    `json:"pull_count"`
		LastUpdated string `json:"last_updated"`
	}
	path := "/v2/repositories/" + h.Username + "/" + h.Repository + "/"
	// Try anonymously first (public repos); with credentials for private ones.
	err = apiclient.DoJSON(ctx, "GET", hubAPI+path, nil, nil, &info)
	if apiclient.IsStatus(err, 404) || apiclient.IsStatus(err, 401) || apiclient.IsStatus(err, 403) {
		sess, serr := NewHubSession(ctx, d)
		if serr != nil {
			if apiclient.IsStatus(err, 404) {
				return &HubRepoInfo{Exists: false}, nil
			}
			return nil, serr
		}
		err = sess.Do(ctx, "GET", path, nil, &info)
	}
	if apiclient.IsStatus(err, 404) {
		return &HubRepoInfo{Exists: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &HubRepoInfo{Exists: true, Private: info.IsPrivate, Pulls: info.PullCount, Updated: info.LastUpdated}, nil
}
