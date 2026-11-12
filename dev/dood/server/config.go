package main

import (
	"os"
	"strconv"
	"strings"
)

// Config is the gateway's view of itself. Every value comes from the
// environment set by entrypoint.sh (root) or by the parent's generated
// compose override (child), so one image serves every level of the cascade.
type Config struct {
	ID              string   // "root" or the dash-joined path of names, e.g. "abc-xyz"
	Prefix          string   // external URL prefix this gateway is mounted at ("" for root)
	Role            string   // root | child
	Isolated        bool     // we run inside our own dind sidecar
	DozzlePath      string   // dozzle for our daemon, when it isn't ours (host bundles)
	Services        []string // local services shown in the console sidebar
	BundleFile      string   // compose file describing one bundle
	RootComposeFile string   // this repo's own top-level docker-compose.yaml
	EnvFile         string   // --env-file handed to compose for interpolation
	RoutesDir       string   // Traefik file-provider directory (watched)
	StateDir        string   // generated per-child override files
	StaticDir       string   // built Vite console
	ProjectsRoot    string   // ~/projects on the host, bind-mounted in (root only)
	WorkspaceFile   string   // workspace.yaml (manager gateways; holds tokens - mounted from outside any repo)
	DeploymentFile  string   // deployment.yaml (Docker Hub images, cloud targets; holds keys - same place)
	WorkspaceName   string   // set in a workspace gateway: the workspace it serves (details arrive over NATS)
	WorkspaceKey    string   // its per-gateway key: unseals the assignment its parent sends
	Listen          string   // address gw-server's HTTP server binds to (Traefik proxies to it)
	NatsLeafPort    int      // leafnode listener, reachable by our parent once it attaches to our gwlink
	NatsStoreDir    string   // JetStream store, backed by a named volume so it survives container recreation
	NatsLeafUser    string   // leafnode auth, set when the leaf port is reachable from outside (a cloud gateway)
	NatsLeafPass    string   // password paired with NatsLeafUser
	NatsClientPort  int      // plain client listener, for ordinary NATS clients on internal_net (e.g. gopher)
}

// getenv returns the environment variable k, or def when it is unset or empty.
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// getenvInt returns the environment variable k parsed as an int, or def when
// it is unset, empty, or not a number.
func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// loadConfig builds the Config from GW_* environment variables, filling in
// the in-container defaults for anything unset.
func loadConfig() Config {
	c := Config{
		ID:              getenv("GW_ID", "root"),
		Prefix:          strings.TrimRight(os.Getenv("GW_PREFIX"), "/"),
		Role:            getenv("GW_ROLE", "root"),
		Isolated:        os.Getenv("GW_ISOLATED") == "true",
		DozzlePath:      os.Getenv("GW_DOZZLE_PATH"),
		BundleFile:      getenv("GW_BUNDLE_FILE", "/self/dev/dood/bundle/compose.yaml"),
		RootComposeFile: getenv("GW_ROOT_COMPOSE_FILE", "/self/docker-compose.yaml"),
		EnvFile:         getenv("GW_ENV_FILE", "/self/.env"),
		RoutesDir:       getenv("GW_ROUTES_DIR", "/etc/traefik/dynamic"),
		StateDir:        getenv("GW_STATE_DIR", "/var/lib/d8zgw"),
		StaticDir:       getenv("GW_STATIC_DIR", "/srv/console"),
		ProjectsRoot:    os.Getenv("GW_PROJECTS_ROOT"),
		WorkspaceFile:   getenv("GW_WORKSPACE_FILE", "/etc/d8z/workspace.yaml"),
		DeploymentFile:  getenv("GW_DEPLOYMENT_FILE", "/etc/d8z/deployment.yaml"),
		WorkspaceName:   os.Getenv("GW_WORKSPACE_NAME"),
		WorkspaceKey:    os.Getenv("GW_WORKSPACE_KEY"),
		Listen:          getenv("GW_LISTEN", "127.0.0.1:8000"),
		NatsLeafPort:    getenvInt("GW_NATS_LEAF_PORT", 7422),
		NatsStoreDir:    getenv("GW_NATS_STORE_DIR", "/var/lib/d8zgw-nats"),
		NatsLeafUser:    os.Getenv("GW_NATS_LEAF_USER"),
		NatsLeafPass:    os.Getenv("GW_NATS_LEAF_PASSWORD"),
		NatsClientPort:  getenvInt("GW_NATS_CLIENT_PORT", 4222),
	}
	for _, s := range strings.Split(getenv("GW_SERVICES", "deightz,og,gopher,dozzle"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.Services = append(c.Services, s)
		}
	}
	return c
}

// Profile is one of the bundle profiles a child can be started with. UI says
// whether it gets a sidebar entry. There are no dependencies between them, with one rule:
// one dozzle per docker daemon. A dind bundle is a new daemon, so it always
// gets one; a host bundle shares its parent's daemon (and its dozzle), so
// it never does.
type Profile struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	UI    bool   `json:"ui"`
}

// profiles is every bundle profile, in canonical order.
var profiles = []Profile{
	{Name: "deightz", Label: "Deightz IDE", UI: true},
	{Name: "og", Label: "Ollama gateway", UI: true},
	{Name: "gopher", Label: "Gopher tests & docs", UI: true},
	{Name: "dozzle", Label: "Dozzle logs", UI: true},
}

// resolveProfiles drops unknown names, applies the dozzle rule for the
// given docker mode, and keeps the canonical order so overrides and labels
// are stable.
func resolveProfiles(req []string, docker string) []string {
	want := map[string]bool{}
	for _, n := range req {
		want[n] = true
	}
	want["dozzle"] = docker == "dind"
	var out []string
	for _, p := range profiles {
		if want[p.Name] {
			out = append(out, p.Name)
		}
	}
	return out
}

// allProfileNames lists every profile name, in canonical order.
func allProfileNames() []string {
	out := make([]string, len(profiles))
	for i, p := range profiles {
		out[i] = p.Name
	}
	return out
}

// uiServices keeps the profiles in profs that get a console sidebar entry.
func uiServices(profs []string) []string {
	var out []string
	for _, n := range profs {
		for _, p := range profiles {
			if p.Name == n && p.UI {
				out = append(out, n)
			}
		}
	}
	return out
}
