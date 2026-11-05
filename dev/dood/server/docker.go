package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// The docker CLI (and its compose plugin) is already in the image for
// bringing bundles up, so the rest of the daemon traffic goes through it
// too: one client, honouring DOCKER_HOST, no SDK dependency.

const (
	labelParent     = "d8z.gw.parent"
	labelID         = "d8z.gw.id"
	labelName       = "d8z.gw.name"
	labelPrefix     = "d8z.gw.prefix"
	labelDocker     = "d8z.gw.docker"
	labelProfiles   = "d8z.gw.profiles"
	labelEntry      = "d8z.gw.entry"      // the container the parent routes to
	labelSrcProject = "d8z.gw.srcproject" // sibling project the bundle was spawned from ("" = this repo)
	labelWorkspace  = "d8z.gw.workspace"  // workspace the gateway serves ("" = none: a plain bundle)
	labelBranch     = "d8z.gw.branch"     // saved config branch it was started from ("" = none)
	labelProject    = "com.docker.compose.project"
	labelService    = "com.docker.compose.service"
)

func dockerOut(ctx context.Context, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		return out, fmt.Errorf("docker %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

// dockerStream runs docker with output copied to w (an op's log).
func dockerStream(ctx context.Context, w io.Writer, args ...string) error {
	return dockerStreamHost(ctx, w, "", args...)
}

// dockerStreamHost is dockerStream against another daemon (via
// DOCKER_HOST, which the compose plugin honours too).
func dockerStreamHost(ctx context.Context, w io.Writer, host string, args ...string) error {
	return dockerStreamHostEnv(ctx, w, host, nil, args...)
}

// dockerStreamHostEnv is dockerStreamHost with extra process-environment
// variables (e.g. "PROJECT=foo") - needed for anything compose resolves by
// variable substitution (${PROJECT} in a volume source or default), which
// it reads from its own env/--env-file, not a container's `environment:`.
func dockerStreamHostEnv(ctx context.Context, w io.Writer, host string, extraEnv []string, args ...string) error {
	if len(extraEnv) > 0 {
		fmt.Fprintf(w, "$ %s docker %s\n", strings.Join(extraEnv, " "), strings.Join(args, " "))
	} else if host != "" {
		fmt.Fprintf(w, "$ DOCKER_HOST=%s docker %s\n", host, strings.Join(args, " "))
	} else {
		fmt.Fprintf(w, "$ docker %s\n", strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if host != "" || len(extraEnv) > 0 {
		env := os.Environ()
		if host != "" {
			env = append(env, "DOCKER_HOST="+host)
		}
		cmd.Env = append(env, extraEnv...)
	}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %v", args[0], err)
	}
	return nil
}

type container struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Status string `json:"Status"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func (c container) name() string { return strings.TrimPrefix(c.Name, "/") }

func inspect(ctx context.Context, ids []string) ([]container, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := dockerOut(ctx, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var cs []container
	return cs, json.Unmarshal([]byte(out), &cs)
}

// childGateways returns the entry containers (a child's Traefik, or a dind
// child's sidecar) whose parent is `parent`.
func childGateways(ctx context.Context, parent string) ([]container, error) {
	out, err := dockerOut(ctx, "ps", "-aq", "--filter", "label="+labelParent+"="+parent)
	if err != nil {
		return nil, err
	}
	all, err := inspect(ctx, strings.Fields(out))
	var res []container
	for _, c := range all {
		l := c.Config.Labels
		// Gateways from before the entry label: their labelled Traefik.
		if l[labelEntry] == "true" || (l[labelEntry] == "" && l[labelService] == "traefik") {
			res = append(res, c)
		}
	}
	return res, err
}

// removeProject deletes every container, network and volume of a compose
// project by label, so it works without the files it was created from.
// Network endpoints are force-disconnected first: the parent gateway is
// attached to the child's gwlink and would otherwise pin it.
func removeProject(ctx context.Context, w io.Writer, project string) error {
	filter := "label=" + labelProject + "=" + project
	var errs []string
	if out, err := dockerOut(ctx, "ps", "-aq", "--filter", filter); err != nil {
		errs = append(errs, err.Error())
	} else if ids := strings.Fields(out); len(ids) > 0 {
		if err := dockerStream(ctx, w, append([]string{"rm", "-fv"}, ids...)...); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if out, err := dockerOut(ctx, "network", "ls", "-q", "--filter", filter); err == nil {
		for _, n := range strings.Fields(out) {
			eps, _ := dockerOut(ctx, "network", "inspect", "-f", "{{range $k, $v := .Containers}}{{$k}} {{end}}", n)
			for _, ep := range strings.Fields(eps) {
				dockerOut(ctx, "network", "disconnect", "-f", n, ep)
			}
			if err := dockerStream(ctx, w, "network", "rm", n); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if out, err := dockerOut(ctx, "volume", "ls", "-q", "--filter", filter); err == nil {
		if vs := strings.Fields(out); len(vs) > 0 {
			if err := dockerStream(ctx, w, append([]string{"volume", "rm", "-f"}, vs...)...); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// ServiceStatus is one container of a bundle as the console sees it.
type ServiceStatus struct {
	Service   string `json:"service"`
	Container string `json:"container"`
	State     string `json:"state"`  // created | running | restarting | exited | dead ...
	Status    string `json:"status"` // docker's summary, e.g. "Exited (1) 3 minutes ago", "Up 2m (healthy)"
}

// withHost prefixes args with -H when talking to a daemon other than
// DOCKER_HOST (a dind child's sidecar).
func withHost(host string, args ...string) []string {
	if host == "" {
		return args
	}
	return append([]string{"-H", host}, args...)
}

// projectServices groups every compose-managed container on a daemon by
// project, in one `docker ps` call.
func projectServices(ctx context.Context, host string) (map[string][]ServiceStatus, error) {
	out, err := dockerOut(ctx, withHost(host, "ps", "-a", "--filter", "label="+labelProject,
		"--format", `{{.Label "`+labelProject+`"}}`+"\t"+`{{.Label "`+labelService+`"}}`+"\t{{.Names}}\t{{.State}}\t{{.Status}}")...)
	if err != nil {
		return nil, err
	}
	res := map[string][]ServiceStatus{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 5)
		if len(f) < 5 {
			continue
		}
		res[f[0]] = append(res[f[0]], ServiceStatus{Service: f[1], Container: f[2], State: f[3], Status: f[4]})
	}
	for _, v := range res {
		sort.Slice(v, func(i, j int) bool { return v[i].Service < v[j].Service })
	}
	return res, nil
}

// serviceLogs returns the tail of one bundle service's container log.
func serviceLogs(ctx context.Context, host, project, service string) (string, error) {
	id, err := dockerOut(ctx, withHost(host, "ps", "-aq", "--filter", "label="+labelProject+"="+project,
		"--filter", "label="+labelService+"="+service)...)
	if err != nil {
		return "", err
	}
	ids := strings.Fields(id)
	if len(ids) == 0 {
		return "", fmt.Errorf("no container for %s/%s", project, service)
	}
	return dockerOut(ctx, withHost(host, "logs", "--tail", "300", "--timestamps", ids[0])...)
}
