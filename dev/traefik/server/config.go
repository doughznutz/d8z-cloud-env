package main

import (
	"os"
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
	Listen          string
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loadConfig() Config {
	c := Config{
		ID:              getenv("GW_ID", "root"),
		Prefix:          strings.TrimRight(os.Getenv("GW_PREFIX"), "/"),
		Role:            getenv("GW_ROLE", "root"),
		Isolated:        os.Getenv("GW_ISOLATED") == "true",
		DozzlePath:      os.Getenv("GW_DOZZLE_PATH"),
		BundleFile:      getenv("GW_BUNDLE_FILE", "/self/dev/traefik/bundle/compose.yaml"),
		RootComposeFile: getenv("GW_ROOT_COMPOSE_FILE", "/self/docker-compose.yaml"),
		EnvFile:         getenv("GW_ENV_FILE", "/self/.env"),
		RoutesDir:       getenv("GW_ROUTES_DIR", "/etc/traefik/dynamic"),
		StateDir:        getenv("GW_STATE_DIR", "/var/lib/d8zgw"),
		StaticDir:       getenv("GW_STATIC_DIR", "/srv/console"),
		ProjectsRoot:    os.Getenv("GW_PROJECTS_ROOT"),
		Listen:          getenv("GW_LISTEN", "127.0.0.1:8000"),
	}
	for _, s := range strings.Split(getenv("GW_SERVICES", "deightz,og,gopher,dozzle"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.Services = append(c.Services, s)
		}
	}
	return c
}

// Bundle profiles a child can be started with. UI says whether it gets a
// sidebar entry. There are no dependencies between them, with one rule:
// one dozzle per docker daemon. A dind bundle is a new daemon, so it always
// gets one; a host bundle shares its parent's daemon (and its dozzle), so
// it never does.
type Profile struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	UI    bool   `json:"ui"`
}

var profiles = []Profile{
	{Name: "deightz", Label: "Deightz IDE", UI: true},
	{Name: "og", Label: "Ollama gateway", UI: true},
	{Name: "gopher", Label: "Gopher test runner", UI: true},
	{Name: "dozzle", Label: "Dozzle logs", UI: true},
	{Name: "otel", Label: "OTel collector"},
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
