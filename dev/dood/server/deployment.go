package main

// deployment.yaml: what this gateway builds and ships, and where it runs it.
// It lives beside workspace.yaml in the config dir (/etc/d8z in here,
// ~/.d8z on the host), outside every repository, because it can hold the
// Docker Hub token and names the GCP service-account key file. It's
// hand-edited and re-read on every use, so a change shows up without a
// restart. With no file, everything defaults from .env (see loadDeployment).
//
//   docker_hub:                  # one repository, one tag per named container
//     username: doughznutz       # default: DOCKER_HUB_USERNAME
//     repository: doughznutz     # default: DOCKER_HUB_REPO
//     token_env: DOCKER_HUB_TOKEN  # or token: dckr_pat_...
//     images:                    # tag -> the *-dh compose service building it
//       - { name: dood, service: dood-dh }
//       - { name: d8z, service: deightz-dh }
//   gcp:
//     project: my-gcp-project
//     zone: us-west1-b
//     credentials: gcp-sa.json   # service-account key, a file in ~/.d8z
//   targets:                     # cloud gateways: a VM running one profile
//     - name: cloud1             # its NATS id (GW_ID); VM d8z-cloud1
//       profile: root            # root | workspace
//       allow_from: [203.0.113.7/32]  # who may reach :80 and the NATS leaf port
//
// Runtime facts about targets (the VM's address) are kept apart, in
// deployment.state.json beside it, so the YAML stays purely yours.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// HubImage is one image we publish: a tag in the Docker Hub repository,
// built by a docker-compose.yaml service.
type HubImage struct {
	Name    string `yaml:"name" json:"name"`       // tag in the repository: <username>/<repository>:<name>
	Service string `yaml:"service" json:"service"` // the docker-compose.yaml service that builds it (a *-dh one)
}

// DockerHubConfig is deployment.yaml's docker_hub section: the account and
// repository our images are pushed to, and how to authenticate.
type DockerHubConfig struct {
	Username   string     `yaml:"username,omitempty" json:"username"`
	Repository string     `yaml:"repository,omitempty" json:"repository"`
	Token      string     `yaml:"token,omitempty" json:"-"`                      // stored token (never sent to the console)
	TokenEnv   string     `yaml:"token_env,omitempty" json:"tokenEnv,omitempty"` // or the env var holding it
	Images     []HubImage `yaml:"images,omitempty" json:"images"`
}

// GCPConfig is deployment.yaml's gcp section: where cloud targets are
// created and the service-account key used to do it.
type GCPConfig struct {
	Project     string `yaml:"project,omitempty" json:"project"`
	Zone        string `yaml:"zone,omitempty" json:"zone"`
	Credentials string `yaml:"credentials,omitempty" json:"credentials"` // key file name in the config dir
}

// CloudTarget is one cloud gateway: a VM running one profile of our dood
// image, reachable over HTTP and as a NATS leaf.
type CloudTarget struct {
	Name        string   `yaml:"name" json:"name"`                                  // its GW_ID; the VM is d8z-<name>
	Provider    string   `yaml:"provider,omitempty" json:"provider"`                // gcp (the only one)
	Profile     string   `yaml:"profile,omitempty" json:"profile"`                  // root | workspace
	Image       string   `yaml:"image,omitempty" json:"image"`                      // docker_hub image the gateway runs (its dood)
	MachineType string   `yaml:"machine_type,omitempty" json:"machineType"`         // GCE machine type
	DiskGB      int      `yaml:"disk_gb,omitempty" json:"diskGB"`                   // boot disk size
	AllowFrom   []string `yaml:"allow_from,omitempty" json:"allowFrom"`             // CIDRs the firewall lets in
	ForwardEnv  []string `yaml:"forward_env,omitempty" json:"forwardEnv,omitempty"` // our env vars passed to the VM
}

// Deployment is the whole of deployment.yaml.
type Deployment struct {
	DockerHub DockerHubConfig `yaml:"docker_hub" json:"dockerHub"`
	GCP       GCPConfig       `yaml:"gcp" json:"gcp"`
	Targets   []CloudTarget   `yaml:"targets" json:"targets"`
}

// The NATS leaf user and port every cloud gateway listens with.
const (
	cloudLeafUser = "d8z"
	cloudLeafPort = 7422
)

// defaultHubImages are the images published when deployment.yaml lists none.
var defaultHubImages = []HubImage{{Name: "dood", Service: "dood-dh"}, {Name: "d8z", Service: "deightz-dh"}}

// loadDeployment reads deployment.yaml (or nothing, if it's absent) and
// fills in every default, so callers never see a blank field.
func (m *Manager) loadDeployment() (*Deployment, bool, error) {
	raw, fromFile, err := m.loadDeploymentRaw()
	if err != nil {
		return nil, fromFile, err
	}
	d := raw.withDefaults()
	return d, fromFile, d.validate()
}

// loadDeploymentRaw is the file exactly as written: what edits start from,
// so saving never bakes defaults into it.
func (m *Manager) loadDeploymentRaw() (*Deployment, bool, error) {
	d := &Deployment{}
	b, err := os.ReadFile(m.cfg.DeploymentFile)
	if errors.Is(err, os.ErrNotExist) {
		return d, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := yaml.Unmarshal(b, d); err != nil {
		return nil, true, fmt.Errorf("%s: %w", m.cfg.DeploymentFile, err)
	}
	return d, true, nil
}

// withDefaults is a copy with every default applied (from .env where one
// exists).
func (raw *Deployment) withDefaults() *Deployment {
	d := *raw
	d.DockerHub.Images = append([]HubImage{}, raw.DockerHub.Images...)
	d.Targets = append([]CloudTarget{}, raw.Targets...)
	h := &d.DockerHub
	h.Username = firstNonEmpty(h.Username, os.Getenv("DOCKER_HUB_USERNAME"))
	h.Repository = firstNonEmpty(h.Repository, os.Getenv("DOCKER_HUB_REPO"))
	if h.Token == "" && h.TokenEnv == "" {
		h.TokenEnv = "DOCKER_HUB_TOKEN"
	}
	if len(h.Images) == 0 {
		h.Images = append([]HubImage{}, defaultHubImages...)
	}
	d.GCP.Project = firstNonEmpty(d.GCP.Project, os.Getenv("CLOUD_PROJECT_ID"))
	d.GCP.Zone = firstNonEmpty(d.GCP.Zone, os.Getenv("COMPUTE_ZONE"))
	for i := range d.Targets {
		t := &d.Targets[i]
		t.Provider = firstNonEmpty(t.Provider, "gcp")
		t.Profile = firstNonEmpty(t.Profile, "root")
		t.Image = firstNonEmpty(t.Image, "dood")
		t.MachineType = firstNonEmpty(t.MachineType, "e2-small")
		if t.DiskGB == 0 {
			t.DiskGB = 20
		}
	}
	return &d
}

// validate checks every field that ends up in a Docker Hub reference, a
// GCE resource, a file name or an env var lookup.
func (d *Deployment) validate() error {
	h := d.DockerHub
	if h.Username != "" && !hubRepoRE.MatchString(h.Username) {
		return fmt.Errorf("docker_hub.username %q is not a valid Docker Hub namespace", h.Username)
	}
	if h.Repository != "" && !hubRepoRE.MatchString(h.Repository) {
		return fmt.Errorf("docker_hub.repository %q: lowercase letters, digits, . _ -", h.Repository)
	}
	if h.TokenEnv != "" && !wsEnvRE.MatchString(h.TokenEnv) {
		return fmt.Errorf("docker_hub.token_env must be an environment variable name")
	}
	seen := map[string]bool{}
	for _, im := range h.Images {
		if !hubTagRE.MatchString(im.Name) || im.Name == "all" {
			return fmt.Errorf("docker_hub.images: invalid image name (tag) %q", im.Name)
		}
		if seen[im.Name] {
			return fmt.Errorf("docker_hub.images: %q listed twice", im.Name)
		}
		seen[im.Name] = true
		if !wsNameRE.MatchString(im.Service) {
			return fmt.Errorf("docker_hub.images[%s]: service must name a docker-compose.yaml service", im.Name)
		}
	}
	if c := d.GCP.Credentials; c != "" && (strings.ContainsAny(c, "/\\") || !strings.HasSuffix(c, ".json") ||
		strings.HasPrefix(c, ".") || c == "deployment.state.json") {
		return fmt.Errorf("gcp.credentials must be a .json file name in the config dir (~/.d8z), not a path")
	}
	names := map[string]bool{}
	for _, t := range d.Targets {
		if !nameRE.MatchString(t.Name) || t.Name == "root" {
			return fmt.Errorf("targets: name %q must match %s (it becomes the gateway's NATS id)", t.Name, nameRE)
		}
		if names[t.Name] {
			return fmt.Errorf("targets: %q listed twice", t.Name)
		}
		names[t.Name] = true
		if t.Provider != "gcp" {
			return fmt.Errorf("targets[%s]: provider %q isn't supported (gcp)", t.Name, t.Provider)
		}
		if t.Profile != "root" && t.Profile != "workspace" {
			return fmt.Errorf("targets[%s]: profile must be root or workspace", t.Name)
		}
		if !seen[t.Image] {
			return fmt.Errorf("targets[%s]: image %q isn't one of docker_hub.images", t.Name, t.Image)
		}
		if !gceNameRE.MatchString(t.MachineType) {
			return fmt.Errorf("targets[%s]: invalid machine_type", t.Name)
		}
		if t.DiskGB < 10 || t.DiskGB > 2000 {
			return fmt.Errorf("targets[%s]: disk_gb must be 10-2000", t.Name)
		}
		for _, c := range t.AllowFrom {
			if _, _, err := net.ParseCIDR(c); err != nil {
				return fmt.Errorf("targets[%s]: allow_from %q is not a CIDR (e.g. 203.0.113.7/32)", t.Name, c)
			}
		}
		for _, k := range t.ForwardEnv {
			if !wsEnvRE.MatchString(k) {
				return fmt.Errorf("targets[%s]: forward_env %q is not a variable name", t.Name, k)
			}
		}
	}
	return nil
}

// image looks up a docker_hub image by name (tag).
func (d *Deployment) image(name string) (HubImage, bool) {
	for _, im := range d.DockerHub.Images {
		if im.Name == name {
			return im, true
		}
	}
	return HubImage{}, false
}

// target looks up a cloud target by name.
func (d *Deployment) target(name string) (CloudTarget, bool) {
	for _, t := range d.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return CloudTarget{}, false
}

// ref is the full Docker Hub reference of one of our images.
func (d *Deployment) ref(name string) string {
	return d.DockerHub.Username + "/" + d.DockerHub.Repository + ":" + name
}

// hubToken is the Docker Hub token: the stored one, else TokenEnv's value.
func (d *Deployment) hubToken() string {
	if t := strings.TrimSpace(d.DockerHub.Token); t != "" {
		return t
	}
	return strings.TrimSpace(os.Getenv(d.DockerHub.TokenEnv))
}

// configDir is the directory deployment.yaml (and the GCP key) live in.
func (d *Deployment) configDir(m *Manager) string { return filepath.Dir(m.cfg.DeploymentFile) }

// ---- runtime state: deployment.state.json ----

// cloudState is what we last learned about one target's VM.
type cloudState struct {
	Instance string    `json:"instance"`
	Zone     string    `json:"zone"`
	Address  string    `json:"address,omitempty"` // external IP, while it has one
	Status   string    `json:"status"`            // GCE status: RUNNING, TERMINATED, ...
	Updated  time.Time `json:"updated"`
}

// cloudStateMu serializes read-modify-write of deployment.state.json.
var cloudStateMu sync.Mutex

// cloudStateFile is deployment.state.json, beside deployment.yaml.
func (m *Manager) cloudStateFile() string {
	return filepath.Join(filepath.Dir(m.cfg.DeploymentFile), "deployment.state.json")
}

// loadCloudState reads every target's state, by target name (empty if the
// file is missing or unreadable).
func (m *Manager) loadCloudState() map[string]cloudState {
	cloudStateMu.Lock()
	defer cloudStateMu.Unlock()
	out := map[string]cloudState{}
	if b, err := os.ReadFile(m.cloudStateFile()); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

// setCloudState records (st != nil) or forgets (nil) one target's state.
func (m *Manager) setCloudState(name string, st *cloudState) error {
	cloudStateMu.Lock()
	defer cloudStateMu.Unlock()
	all := map[string]cloudState{}
	if b, err := os.ReadFile(m.cloudStateFile()); err == nil {
		json.Unmarshal(b, &all)
	}
	if st == nil {
		delete(all, name)
	} else {
		st.Updated = time.Now().UTC()
		all[name] = *st
	}
	b, _ := json.MarshalIndent(all, "", "  ")
	return os.WriteFile(m.cloudStateFile(), b, 0o600)
}

// cloudLeafPassword is the NATS leaf password of cloud target name,
// derived from our master key: nothing to store, and only we (and the VM we
// hand it to) know it.
func (m *Manager) cloudLeafPassword(name string) string {
	return m.childKey("cloud:" + name)[:40]
}

// cloudLeafURLs are the leaf remotes for our running cloud gateways - only
// targets still in deployment.yaml, with a known address.
func (m *Manager) cloudLeafURLs() map[string]string {
	out := map[string]string{}
	d, _, err := m.loadDeployment()
	if err != nil {
		return out
	}
	for name, st := range m.loadCloudState() {
		if _, ok := d.target(name); !ok || st.Address == "" || st.Status != "RUNNING" {
			continue
		}
		out[name] = fmt.Sprintf("nats-leaf://%s:%s@%s", cloudLeafUser, m.cloudLeafPassword(name),
			net.JoinHostPort(st.Address, fmt.Sprint(cloudLeafPort)))
	}
	return out
}

// ---- HTTP ----

// registerDeploymentRoutes adds the Deployment tab's read-only view of
// deployment.yaml (with defaults, key status and each target's live state)
// and the endpoint for messaging a cloud gateway.
func registerDeploymentRoutes(mux *http.ServeMux, m *Manager) {
	mux.HandleFunc("GET /api/deployment", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"file": m.cfg.DeploymentFile, "editable": true}
		d, fromFile, err := m.loadDeployment()
		resp["fromFile"] = fromFile
		if err != nil {
			resp["error"] = err.Error()
		}
		if d == nil {
			writeJSON(w, http.StatusOK, resp)
			return
		}
		type imageView struct {
			HubImage
			Ref string `json:"ref"`
		}
		var images []imageView
		for _, im := range d.DockerHub.Images {
			images = append(images, imageView{im, d.ref(im.Name)})
		}
		resp["dockerHub"] = map[string]any{
			"username": d.DockerHub.Username, "repository": d.DockerHub.Repository,
			"tokenSet": d.hubToken() != "", "tokenStored": d.DockerHub.Token != "", "tokenEnv": d.DockerHub.TokenEnv, "images": images,
		}
		credsOK := false
		if d.GCP.Credentials != "" {
			_, cerr := os.Stat(filepath.Join(d.configDir(m), d.GCP.Credentials))
			credsOK = cerr == nil
		}
		resp["gcp"] = map[string]any{"project": d.GCP.Project, "zone": d.GCP.Zone,
			"credentials": d.GCP.Credentials, "credentialsOK": credsOK, "keyEmail": m.gcpKeyEmail(d)}
		if resp["editable"] == true {
			resp["dhServices"] = m.dhServices(r.Context())
			resp["publicIP"] = publicIP(r.Context())
		}

		state := m.loadCloudState()
		leafs := map[string]bool{}
		if m.nats != nil {
			leafs = m.nats.LeafRemotes()
		}
		var targets []map[string]any
		for _, t := range d.Targets {
			v := map[string]any{"target": t, "ref": d.ref(t.Image)}
			if st, ok := state[t.Name]; ok {
				v["state"] = st
				if st.Address != "" {
					v["url"] = "http://" + st.Address + "/"
					v["leafConnected"] = leafs[net.JoinHostPort(st.Address, fmt.Sprint(cloudLeafPort))]
				}
			}
			targets = append(targets, v)
		}
		resp["targets"] = targets
		writeJSON(w, http.StatusOK, resp)
	})

	// Send a message to a cloud gateway over NATS: published into its inbox
	// (it crosses our leaf link to the VM), then a ping round trip proves it
	// arrived and its gw-server is answering.
	mux.HandleFunc("POST /api/deployment/targets/{name}/message", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := m.messageCloud(r.PathValue("name"), req.Text)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

// messageCloud publishes text into cloud target name's inbox, then pings
// it to confirm it arrived, returning the ping reply.
func (m *Manager) messageCloud(name, text string) (map[string]string, error) {
	if m.nats == nil {
		return nil, fmt.Errorf("nats not available")
	}
	d, _, err := m.loadDeployment()
	if err != nil {
		return nil, err
	}
	if _, ok := d.target(name); !ok {
		return nil, fmt.Errorf("no target %q in deployment.yaml", name)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty message")
	}
	if err := m.nats.Publish(name, text); err != nil {
		return nil, err
	}
	reply, err := m.nats.Ping(name, text, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("sent, but %s didn't answer a ping over NATS: %v (is its leaf link up?)", name, err)
	}
	return map[string]string{"state": "delivered", "reply": reply}, nil
}
