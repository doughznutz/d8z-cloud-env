package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cfgManager is a Manager whose config dir is a fresh temp dir.
func cfgManager(t *testing.T) *Manager {
	dir := t.TempDir()
	wsf := filepath.Join(dir, "workspace.yaml")
	return &Manager{cfg: Config{DeploymentFile: filepath.Join(dir, "deployment.yaml"), WorkspaceFile: wsf}, ws: &wsStore{file: wsf}}
}

// writeCfg writes files (name -> content) into m's config dir.
func writeCfg(t *testing.T, m *Manager, files map[string]string) {
	for f, s := range files {
		if err := os.WriteFile(filepath.Join(m.cfgDir(), f), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// readCfg reads file f from m's config dir.
func readCfg(t *testing.T, m *Manager, f string) string {
	b, err := os.ReadFile(filepath.Join(m.cfgDir(), f))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// parentWS is the parent's workspace.yaml the config tests start from.
const parentWS = `workspaces:
  - name: alpha
    source: github
    repo: me/alpha
  - name: beta
    source: local
    path: beta
`

// A child takes its parent's config, branches it, rebases onto the parent's
// later changes, and the parent can merge the branch back - or save it and
// start a new child from it.
func TestConfigBranchRebaseMerge(t *testing.T) {
	parent, child := cfgManager(t), cfgManager(t)
	writeCfg(t, parent, map[string]string{"workspace.yaml": parentWS, "gcp-sa.json": `{"k":1}`, "deployment.state.json": `{}`})

	give := func() {
		files, env, err := parent.configGift()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := files["deployment.state.json"]; ok {
			t.Fatal("runtime state was passed down")
		}
		if err := child.receiveConfig(files, env, nil); err != nil {
			t.Fatal(err)
		}
	}
	give()
	if ops, _ := child.configDiff(); len(ops) != 0 {
		t.Fatalf("fresh child has changes: %v", ops)
	}
	if !strings.Contains(readCfg(t, child, "workspace.yaml"), "me/alpha") {
		t.Fatal("child didn't get the parent's workspaces")
	}

	// The child edits alpha's branch and removes beta.
	if err := child.ws.update(func(l []Workspace) ([]Workspace, error) {
		out := []Workspace{}
		for _, w := range l {
			if w.Name == "alpha" {
				w.Branch = "dev"
			}
			if w.Name != "beta" {
				out = append(out, w)
			}
		}
		return out, nil
	}); err != nil {
		t.Fatal(err)
	}
	ops, err := child.configDiff()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, o := range ops {
		got = append(got, o.Summary())
	}
	if strings.Join(got, "; ") != "set workspace.yaml: workspaces.[alpha].branch; remove workspace.yaml: workspaces.[beta]" {
		t.Fatalf("diff: %v", got)
	}

	// The parent adds gamma: the child rebases, keeping its own changes.
	writeCfg(t, parent, map[string]string{"workspace.yaml": parentWS + "  - name: gamma\n    source: github\n    repo: me/gamma\n"})
	give()
	ws := readCfg(t, child, "workspace.yaml")
	if !strings.Contains(ws, "me/gamma") || !strings.Contains(ws, "branch: dev") || strings.Contains(ws, "beta") {
		t.Fatalf("rebase lost something:\n%s", ws)
	}
	if !strings.HasPrefix(ws, "# D8Z workspaces") {
		t.Fatal("rebased workspace.yaml lost its header")
	}

	// Merged into the parent, the parent has the child's changes.
	ops, _ = child.configDiff()
	if err := parent.mergeConfig(ops); err != nil {
		t.Fatal(err)
	}
	pw := readCfg(t, parent, "workspace.yaml")
	if !strings.Contains(pw, "branch: dev") || strings.Contains(pw, "beta") || !strings.Contains(pw, "gamma") {
		t.Fatalf("merge:\n%s", pw)
	}

	// Or saved as a branch, and a new child started from it.
	id, err := parent.saveBranch("kid", ops)
	if err != nil {
		t.Fatal(err)
	}
	if bs := parent.listBranches(); len(bs) != 1 || bs[0].ID != id || bs[0].Ops != nil || len(bs[0].Changes) != 2 {
		t.Fatalf("branches: %+v", bs)
	}
	br, err := parent.loadBranch(id)
	if err != nil {
		t.Fatal(err)
	}
	writeCfg(t, parent, map[string]string{"workspace.yaml": parentWS})
	kid := cfgManager(t)
	files, env, _ := parent.configGift()
	if _, ok := files["branches/"+id+".json"]; !ok {
		t.Fatal("saved branches don't travel with the config")
	}
	if err := kid.receiveConfig(files, env, br.Ops); err != nil {
		t.Fatal(err)
	}
	kw := readCfg(t, kid, "workspace.yaml")
	if !strings.Contains(kw, "branch: dev") || strings.Contains(kw, "beta") {
		t.Fatalf("child started from a branch:\n%s", kw)
	}
	if ops, _ := kid.configDiff(); len(ops) != 2 {
		t.Fatalf("a child started from a branch should carry it as its changes: %v", ops)
	}
}

// TestConfigGiftCarriesNamedEnv checks that a config handed to a child
// carries the values of the env vars its token_env keys name.
func TestConfigGiftCarriesNamedEnv(t *testing.T) {
	m := cfgManager(t)
	t.Setenv("MY_TOKEN_FOR_TEST", "s3cret")
	writeCfg(t, m, map[string]string{"workspace.yaml": "workspaces:\n  - name: a\n    source: github\n    repo: x/a\n    token_env: MY_TOKEN_FOR_TEST\n"})
	_, env, err := m.configGift()
	if err != nil {
		t.Fatal(err)
	}
	if env["MY_TOKEN_FOR_TEST"] != "s3cret" {
		t.Fatalf("env: %v", env)
	}
}
