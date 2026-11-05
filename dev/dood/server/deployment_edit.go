package main

// Editing deployment.yaml from the console (Deployment tab), the way the
// Workspaces tab edits workspace.yaml: one form per section, secrets
// write-only (set / replace / clear, never sent back), and every save
// validated as a whole before it's written. Only a manager gateway edits -
// a workspace gateway is shown the root's settings, not its own.

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const deploymentFileHeader = `# D8Z deployment - managed by the gateway console (Deployment tab).
# What this gateway builds and ships (named images, one tag each in one
# Docker Hub repository) and where it runs them (cloud targets).
# Can hold the Docker Hub token: keep it outside every repository.
`

var deployMu sync.Mutex

// updateDeployment applies fn to the file as written, validates the result
// with defaults applied, and saves it.
func (m *Manager) updateDeployment(fn func(d *Deployment) error) error {
	deployMu.Lock()
	defer deployMu.Unlock()
	d, _, err := m.loadDeploymentRaw()
	if err != nil {
		return err
	}
	if err := fn(d); err != nil {
		return err
	}
	if err := d.withDefaults().validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(d); err != nil {
		return err
	}
	return writeConfigFile(m.cfg.DeploymentFile, append([]byte(deploymentFileHeader+"\n"), buf.Bytes()...))
}

// ---- inputs ----

type hubInput struct {
	Username   string `json:"username"`
	Repository string `json:"repository"`
	TokenEnv   string `json:"tokenEnv"`
	Token      string `json:"token"`      // a new token ("" keeps the stored one)
	ClearToken bool   `json:"clearToken"` // drop the stored token
}

type gcpInput struct {
	Project     string `json:"project"`
	Zone        string `json:"zone"`
	Credentials string `json:"credentials"` // key file name in the config dir
	KeyJSON     string `json:"keyJSON"`     // a new service-account key ("" keeps the file)
	ClearKey    bool   `json:"clearKey"`    // delete the key file
}

func trimAll(ss []string) []string {
	out := []string{}
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (m *Manager) editDockerHub(in hubInput) error {
	return m.updateDeployment(func(d *Deployment) error {
		h := &d.DockerHub
		h.Username = strings.TrimSpace(in.Username)
		h.Repository = strings.TrimSpace(in.Repository)
		h.TokenEnv = strings.TrimSpace(in.TokenEnv)
		if in.ClearToken {
			h.Token = ""
		}
		if t := strings.TrimSpace(in.Token); t != "" {
			h.Token = t
		}
		return nil
	})
}

// Images: name is the tag. Adding the first one to a file that had none
// keeps the defaults it was showing.
func (m *Manager) putImage(oldName string, im HubImage) error {
	im.Name, im.Service = strings.TrimSpace(im.Name), strings.TrimSpace(im.Service)
	return m.updateDeployment(func(d *Deployment) error {
		if len(d.DockerHub.Images) == 0 {
			d.DockerHub.Images = append([]HubImage{}, defaultHubImages...)
		}
		list := d.DockerHub.Images
		for _, o := range list {
			if o.Name == im.Name && o.Name != oldName {
				return fmt.Errorf("image %q already exists", im.Name)
			}
		}
		if oldName == "" {
			d.DockerHub.Images = append(list, im)
			return nil
		}
		for i, o := range list {
			if o.Name == oldName {
				list[i] = im
				// A renamed tag: targets running it follow.
				for k := range d.Targets {
					if d.Targets[k].Image == oldName {
						d.Targets[k].Image = im.Name
					}
				}
				return nil
			}
		}
		return fmt.Errorf("no image %q", oldName)
	})
}

func (m *Manager) removeImage(name string) error {
	return m.updateDeployment(func(d *Deployment) error {
		if len(d.DockerHub.Images) == 0 {
			d.DockerHub.Images = append([]HubImage{}, defaultHubImages...)
		}
		for _, t := range d.withDefaults().Targets {
			if t.Image == name {
				return fmt.Errorf("target %q runs %q: change or remove it first", t.Name, name)
			}
		}
		out := []HubImage{}
		found := false
		for _, o := range d.DockerHub.Images {
			if o.Name == name {
				found = true
				continue
			}
			out = append(out, o)
		}
		if !found {
			return fmt.Errorf("no image %q", name)
		}
		if len(out) == 0 {
			return fmt.Errorf("keep at least one image")
		}
		d.DockerHub.Images = out
		return nil
	})
}

// checkGCPKey validates a pasted service-account key and returns its email.
func checkGCPKey(b []byte) (string, error) {
	var key struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(b, &key); err != nil || key.Type != "service_account" || key.ClientEmail == "" {
		return "", fmt.Errorf("that isn't a GCP service-account key (JSON with \"type\": \"service_account\")")
	}
	blk, _ := pem.Decode([]byte(key.PrivateKey))
	if blk == nil {
		return "", fmt.Errorf("the key's private_key isn't PEM")
	}
	if _, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err != nil {
		return "", fmt.Errorf("the key's private_key: %v", err)
	}
	return key.ClientEmail, nil
}

func (m *Manager) editGCP(in gcpInput) error {
	creds := firstNonEmpty(strings.TrimSpace(in.Credentials), "gcp-sa.json")
	var key []byte
	if k := strings.TrimSpace(in.KeyJSON); k != "" {
		if _, err := checkGCPKey([]byte(k)); err != nil {
			return err
		}
		key = []byte(k + "\n")
	}
	var keyFile string
	err := m.updateDeployment(func(d *Deployment) error {
		d.GCP.Project = strings.TrimSpace(in.Project)
		d.GCP.Zone = strings.TrimSpace(in.Zone)
		d.GCP.Credentials = creds
		if !gceNameRE.MatchString(firstNonEmpty(d.GCP.Zone, "z")) {
			return fmt.Errorf("invalid zone %q", d.GCP.Zone)
		}
		keyFile = filepath.Join(filepath.Dir(m.cfg.DeploymentFile), creds)
		return nil
	})
	if err != nil {
		return err
	}
	if in.ClearKey {
		if err := os.Remove(keyFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if key != nil {
		return writeConfigFile(keyFile, key)
	}
	return nil
}

func (m *Manager) putTarget(oldName string, t CloudTarget) error {
	t.Name = strings.TrimSpace(t.Name)
	t.MachineType = strings.TrimSpace(t.MachineType)
	t.AllowFrom = trimAll(t.AllowFrom)
	t.ForwardEnv = trimAll(t.ForwardEnv)
	t.Provider = "" // gcp, the only one: keep the file minimal
	return m.updateDeployment(func(d *Deployment) error {
		for _, o := range d.Targets {
			if o.Name == t.Name && o.Name != oldName {
				return fmt.Errorf("target %q already exists", t.Name)
			}
		}
		if oldName == "" {
			d.Targets = append(d.Targets, t)
			return nil
		}
		for i, o := range d.Targets {
			if o.Name != oldName {
				continue
			}
			if t.Name != oldName {
				if st, ok := m.loadCloudState()[oldName]; ok && st.Instance != "" {
					return fmt.Errorf("%s has a VM (%s): delete it (cloud/delete) before renaming the target", oldName, st.Instance)
				}
			}
			d.Targets[i] = t
			return nil
		}
		return fmt.Errorf("no target %q", oldName)
	})
}

func (m *Manager) removeTarget(name string) error {
	if st, ok := m.loadCloudState()[name]; ok && st.Instance != "" {
		return fmt.Errorf("%s still has a VM (%s, %s): run cloud/delete first", name, st.Instance, st.Status)
	}
	return m.updateDeployment(func(d *Deployment) error {
		out := []CloudTarget{}
		for _, o := range d.Targets {
			if o.Name != name {
				out = append(out, o)
			}
		}
		if len(out) == len(d.Targets) {
			return fmt.Errorf("no target %q", name)
		}
		d.Targets = out
		return nil
	})
}

// ---- helpers for the forms ----

// dhServices are the docker-compose.yaml services in the dh profile (what an
// image can be built by), from the project's live checkout. Cached briefly.
var dhCache struct {
	sync.Mutex
	at   time.Time
	list []string
}

func (m *Manager) dhServices(ctx context.Context) []string {
	dhCache.Lock()
	defer dhCache.Unlock()
	if time.Since(dhCache.at) < 30*time.Second && dhCache.list != nil {
		return dhCache.list
	}
	file := m.cfg.RootComposeFile
	if p := defaultProject(); p != "" && m.cfg.ProjectsRoot != "" {
		if f := filepath.Join(m.cfg.ProjectsRoot, p, "docker-compose.yaml"); fileExists(f) {
			file = f
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "--project-directory", filepath.Dir(file), "-f", file,
		"--profile", "dh", "config", "--format", "json")
	out, err := cmd.Output()
	list := []string{}
	if err == nil {
		var cfg struct {
			Services map[string]struct {
				Profiles []string `json:"profiles"`
			} `json:"services"`
		}
		if json.Unmarshal(out, &cfg) == nil {
			for name, s := range cfg.Services {
				for _, p := range s.Profiles {
					if p == "dh" {
						list = append(list, name)
					}
				}
			}
		}
	}
	sort.Strings(list)
	dhCache.at, dhCache.list = time.Now(), list
	return list
}

func fileExists(f string) bool {
	_, err := os.Stat(f)
	return err == nil
}

// publicIP is this gateway's address as the internet sees it - what a
// target's allow_from needs so we can reach (and NATS-dial) the VM.
var ipCache struct {
	sync.Mutex
	at time.Time
	ip string
}

func publicIP(ctx context.Context) string {
	ipCache.Lock()
	defer ipCache.Unlock()
	if ipCache.ip != "" && time.Since(ipCache.at) < 10*time.Minute {
		return ipCache.ip
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ipCache.ip
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if ip := strings.TrimSpace(string(b)); resp.StatusCode == 200 && ip != "" {
		ipCache.ip, ipCache.at = ip, time.Now()
	}
	return ipCache.ip
}

// gcpKeyEmail is the service account a stored key belongs to ("" if none).
func (m *Manager) gcpKeyEmail(d *Deployment) string {
	if d.GCP.Credentials == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(d.configDir(m), d.GCP.Credentials))
	if err != nil {
		return ""
	}
	email, _ := checkGCPKey(b)
	return email
}

// hubRepoStatus asks Docker Hub about the configured repository.
func (m *Manager) hubRepoStatus(ctx context.Context) (map[string]any, error) {
	d, _, err := m.loadDeployment()
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
	err = doJSON(ctx, "GET", hubAPI+path, nil, nil, &info)
	if isStatus(err, 404) || isStatus(err, 401) || isStatus(err, 403) {
		s, serr := m.newHubSession(ctx)
		if serr != nil {
			if isStatus(err, 404) {
				return map[string]any{"exists": false}, nil
			}
			return nil, serr
		}
		err = s.do(ctx, "GET", path, nil, &info)
	}
	if isStatus(err, 404) {
		return map[string]any{"exists": false}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"exists": true, "private": info.IsPrivate, "pulls": info.PullCount, "updated": info.LastUpdated}, nil
}

// ---- HTTP ----

func registerDeploymentEditRoutes(mux *http.ServeMux, m *Manager) {
	decode := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return false
		}
		return true
	}
	done := func(w http.ResponseWriter, err error) {
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"state": "saved"})
	}
	mux.HandleFunc("PUT /api/deployment/docker-hub", func(w http.ResponseWriter, r *http.Request) {
		var in hubInput
		if decode(w, r, &in) {
			done(w, m.editDockerHub(in))
		}
	})
	mux.HandleFunc("GET /api/deployment/docker-hub/repo", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		st, err := m.hubRepoStatus(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/deployment/images", func(w http.ResponseWriter, r *http.Request) {
		var in HubImage
		if decode(w, r, &in) {
			done(w, m.putImage("", in))
		}
	})
	mux.HandleFunc("PUT /api/deployment/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in HubImage
		if decode(w, r, &in) {
			done(w, m.putImage(r.PathValue("name"), in))
		}
	})
	mux.HandleFunc("DELETE /api/deployment/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		done(w, m.removeImage(r.PathValue("name")))
	})
	mux.HandleFunc("PUT /api/deployment/gcp", func(w http.ResponseWriter, r *http.Request) {
		var in gcpInput
		if decode(w, r, &in) {
			done(w, m.editGCP(in))
		}
	})
	mux.HandleFunc("POST /api/deployment/targets", func(w http.ResponseWriter, r *http.Request) {
		var in CloudTarget
		if decode(w, r, &in) {
			done(w, m.putTarget("", in))
		}
	})
	mux.HandleFunc("PUT /api/deployment/targets/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in CloudTarget
		if decode(w, r, &in) {
			done(w, m.putTarget(r.PathValue("name"), in))
		}
	})
	mux.HandleFunc("DELETE /api/deployment/targets/{name}", func(w http.ResponseWriter, r *http.Request) {
		done(w, m.removeTarget(r.PathValue("name")))
	})
}
