package deployment

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

	"d8z/gw/cfgfile"
	"d8z/gw/cfgtree"
	"d8z/gw/internal/apiclient"

	"gopkg.in/yaml.v3"
)

// deploymentFileHeader is written at the top of every deployment.yaml we save.
const deploymentFileHeader = `# D8Z deployment - managed by the gateway console (Deployment tab).
# What this gateway builds and ships (named images, one tag each in one
# Docker Hub repository) and where it runs them (cloud targets).
# Can hold the Docker Hub token: keep it outside every repository.
`

// Render renders tree t back to deployment.yaml's own format (field order,
// header) - the callback main's config branch/merge engine (cfgtree) uses
// for "deployment.yaml".
func Render(t any) ([]byte, error) {
	return cfgtree.RenderAs("deployment.yaml", t, deploymentFileHeader, &Deployment{})
}

// deployMu serializes read-modify-write of deployment.yaml.
var deployMu sync.Mutex

// Update applies fn to the file as written, validates the result with
// defaults applied, and saves it.
func (s *Store) Update(fn func(d *Deployment) error) error {
	deployMu.Lock()
	defer deployMu.Unlock()
	d, _, err := s.loadRaw()
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
	return cfgfile.WriteFile(s.file, append([]byte(deploymentFileHeader+"\n"), buf.Bytes()...))
}

// ---- inputs ----

// HubInput is the Docker Hub form as the console submits it.
type HubInput struct {
	Username   string `json:"username"`
	Repository string `json:"repository"`
	TokenEnv   string `json:"tokenEnv"`
	Token      string `json:"token"`      // a new token ("" keeps the stored one)
	ClearToken bool   `json:"clearToken"` // drop the stored token
}

// GCPInput is the GCP form as the console submits it.
type GCPInput struct {
	Project     string `json:"project"`
	Zone        string `json:"zone"`
	Credentials string `json:"credentials"` // key file name in the config dir
	KeyJSON     string `json:"keyJSON"`     // a new service-account key ("" keeps the file)
	ClearKey    bool   `json:"clearKey"`    // delete the key file
}

// trimAll trims every string in ss and drops the empty ones (never nil).
func trimAll(ss []string) []string {
	out := []string{}
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---- named-list CRUD shared by images and targets ----

// upsertNamed returns a copy of list with item added (oldName == "") or
// replacing the entry named oldName, refusing a name already used by a
// different entry and erroring if oldName isn't found. kind names the
// entries in error messages ("image", "target").
func upsertNamed[T any](list []T, name func(T) string, oldName string, item T, kind string) ([]T, error) {
	n := name(item)
	for _, o := range list {
		if name(o) == n && name(o) != oldName {
			return nil, fmt.Errorf("%s %q already exists", kind, n)
		}
	}
	if oldName == "" {
		return append(list, item), nil
	}
	for i, o := range list {
		if name(o) == oldName {
			out := append([]T{}, list...)
			out[i] = item
			return out, nil
		}
	}
	return nil, fmt.Errorf("no %s %q", kind, oldName)
}

// removeNamed returns a copy of list with the entry named target dropped,
// or an error if none matched.
func removeNamed[T any](list []T, name func(T) string, target string, kind string) ([]T, error) {
	out := make([]T, 0, len(list))
	found := false
	for _, o := range list {
		if name(o) == target {
			found = true
			continue
		}
		out = append(out, o)
	}
	if !found {
		return nil, fmt.Errorf("no %s %q", kind, target)
	}
	return out, nil
}

// EditDockerHub saves the docker_hub account settings. The stored token is
// replaced only by a non-empty new one, or cleared on request.
func (s *Store) EditDockerHub(in HubInput) error {
	return s.Update(func(d *Deployment) error {
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

// PutImage adds an image (oldName "") or replaces image oldName. The name
// is the tag. Adding the first one to a file that had none keeps the
// defaults it was showing; renaming one moves the targets that run it along.
func (s *Store) PutImage(oldName string, im HubImage) error {
	im.Name, im.Service = strings.TrimSpace(im.Name), strings.TrimSpace(im.Service)
	return s.Update(func(d *Deployment) error {
		if len(d.DockerHub.Images) == 0 {
			d.DockerHub.Images = append([]HubImage{}, defaultHubImages...)
		}
		out, err := upsertNamed(d.DockerHub.Images, func(i HubImage) string { return i.Name }, oldName, im, "image")
		if err != nil {
			return err
		}
		d.DockerHub.Images = out
		if oldName != "" && im.Name != oldName {
			// A renamed tag: targets running it follow.
			for k := range d.Targets {
				if d.Targets[k].Image == oldName {
					d.Targets[k].Image = im.Name
				}
			}
		}
		return nil
	})
}

// RemoveImage drops an image, refusing while a target runs it or if it is
// the last one.
func (s *Store) RemoveImage(name string) error {
	return s.Update(func(d *Deployment) error {
		if len(d.DockerHub.Images) == 0 {
			d.DockerHub.Images = append([]HubImage{}, defaultHubImages...)
		}
		for _, t := range d.withDefaults().Targets {
			if t.Image == name {
				return fmt.Errorf("target %q runs %q: change or remove it first", t.Name, name)
			}
		}
		out, err := removeNamed(d.DockerHub.Images, func(i HubImage) string { return i.Name }, name, "image")
		if err != nil {
			return err
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

// EditGCP saves the gcp settings and, when given, validates and writes a
// new service-account key file (or deletes the old one on request).
func (s *Store) EditGCP(in GCPInput) error {
	creds := firstNonEmpty(strings.TrimSpace(in.Credentials), "gcp-sa.json")
	var key []byte
	if k := strings.TrimSpace(in.KeyJSON); k != "" {
		if _, err := checkGCPKey([]byte(k)); err != nil {
			return err
		}
		key = []byte(k + "\n")
	}
	var keyFile string
	err := s.Update(func(d *Deployment) error {
		d.GCP.Project = strings.TrimSpace(in.Project)
		d.GCP.Zone = strings.TrimSpace(in.Zone)
		d.GCP.Credentials = creds
		if !gceNameRE.MatchString(firstNonEmpty(d.GCP.Zone, "z")) {
			return fmt.Errorf("invalid zone %q", d.GCP.Zone)
		}
		keyFile = filepath.Join(s.Dir(), creds)
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
		return cfgfile.WriteFile(keyFile, key)
	}
	return nil
}

// PutTarget adds a target (oldName "") or replaces target oldName. A target
// that still has a VM can't be renamed: the VM is named after it.
func (s *Store) PutTarget(oldName string, t CloudTarget) error {
	t.Name = strings.TrimSpace(t.Name)
	t.MachineType = strings.TrimSpace(t.MachineType)
	t.AllowFrom = trimAll(t.AllowFrom)
	t.ForwardEnv = trimAll(t.ForwardEnv)
	t.Provider = "" // gcp, the only one: keep the file minimal
	if oldName != "" && t.Name != oldName {
		if st, ok := s.LoadState()[oldName]; ok && st.Instance != "" {
			return fmt.Errorf("%s has a VM (%s): delete it (cloud/delete) before renaming the target", oldName, st.Instance)
		}
	}
	return s.Update(func(d *Deployment) error {
		out, err := upsertNamed(d.Targets, func(t CloudTarget) string { return t.Name }, oldName, t, "target")
		if err != nil {
			return err
		}
		d.Targets = out
		return nil
	})
}

// RemoveTarget drops a target, refusing while it still has a VM.
func (s *Store) RemoveTarget(name string) error {
	if st, ok := s.LoadState()[name]; ok && st.Instance != "" {
		return fmt.Errorf("%s still has a VM (%s, %s): run cloud/delete first", name, st.Instance, st.Status)
	}
	return s.Update(func(d *Deployment) error {
		out, err := removeNamed(d.Targets, func(t CloudTarget) string { return t.Name }, name, "target")
		if err != nil {
			return err
		}
		d.Targets = out
		return nil
	})
}

// ---- helpers for the forms ----

// dhCache holds DHServices' last answer and when it was taken.
var dhCache struct {
	sync.Mutex
	at   time.Time
	list []string
}

// defaultProject is the .env's PROJECT - what "this repo" means on the host.
func defaultProject() string { return strings.TrimSpace(os.Getenv("PROJECT")) }

// DHServices are the docker-compose.yaml services in the dh profile (what
// an image can be built by), from the project's live checkout. Cached
// briefly.
func (s *Store) DHServices(ctx context.Context) []string {
	dhCache.Lock()
	defer dhCache.Unlock()
	if time.Since(dhCache.at) < 30*time.Second && dhCache.list != nil {
		return dhCache.list
	}
	file := s.rootComposeFile
	if p := defaultProject(); p != "" && s.projectsRoot != "" {
		if f := filepath.Join(s.projectsRoot, p, "docker-compose.yaml"); fileExists(f) {
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

// fileExists reports whether f can be stat'ed.
func fileExists(f string) bool {
	_, err := os.Stat(f)
	return err == nil
}

// ipCache holds PublicIP's last answer and when it was taken.
var ipCache struct {
	sync.Mutex
	at time.Time
	ip string
}

// PublicIP is this gateway's address as the internet sees it - what a
// target's allow_from needs so we can reach (and NATS-dial) the VM. Cached
// for 10 minutes; on a lookup failure the last known address (or "") is
// returned.
func PublicIP(ctx context.Context) string {
	ipCache.Lock()
	defer ipCache.Unlock()
	if ipCache.ip != "" && time.Since(ipCache.at) < 10*time.Minute {
		return ipCache.ip
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org", nil)
	resp, err := apiclient.HTTPClient.Do(req)
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

// GCPKeyEmail is the service account a stored key belongs to ("" if none).
func (s *Store) GCPKeyEmail(d *Deployment) string {
	if d.GCP.Credentials == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(s.Dir(), d.GCP.Credentials))
	if err != nil {
		return ""
	}
	email, _ := checkGCPKey(b)
	return email
}

// ---- HTTP ----

// RegisterEditRoutes adds the Deployment tab's edit endpoints:
// Docker Hub settings and repository status, images, GCP settings, and
// targets.
func RegisterEditRoutes(mux *http.ServeMux, s *Store) {
	decode := func(w http.ResponseWriter, r *http.Request, v any) bool {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
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
		var in HubInput
		if decode(w, r, &in) {
			done(w, s.EditDockerHub(in))
		}
	})
	mux.HandleFunc("GET /api/deployment/docker-hub/repo", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		st, err := s.HubRepoStatus(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/deployment/images", func(w http.ResponseWriter, r *http.Request) {
		var in HubImage
		if decode(w, r, &in) {
			done(w, s.PutImage("", in))
		}
	})
	mux.HandleFunc("PUT /api/deployment/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in HubImage
		if decode(w, r, &in) {
			done(w, s.PutImage(r.PathValue("name"), in))
		}
	})
	mux.HandleFunc("DELETE /api/deployment/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		done(w, s.RemoveImage(r.PathValue("name")))
	})
	mux.HandleFunc("PUT /api/deployment/gcp", func(w http.ResponseWriter, r *http.Request) {
		var in GCPInput
		if decode(w, r, &in) {
			done(w, s.EditGCP(in))
		}
	})
	mux.HandleFunc("POST /api/deployment/targets", func(w http.ResponseWriter, r *http.Request) {
		var in CloudTarget
		if decode(w, r, &in) {
			done(w, s.PutTarget("", in))
		}
	})
	mux.HandleFunc("PUT /api/deployment/targets/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in CloudTarget
		if decode(w, r, &in) {
			done(w, s.PutTarget(r.PathValue("name"), in))
		}
	})
	mux.HandleFunc("DELETE /api/deployment/targets/{name}", func(w http.ResponseWriter, r *http.Request) {
		done(w, s.RemoveTarget(r.PathValue("name")))
	})
}
