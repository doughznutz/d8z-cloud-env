package main

// Config passed down the tree, branch/merge style.
//
// A gateway's config is the set of files in its config dir (workspace.yaml,
// deployment.yaml, key files, saved branches/). A child is given its
// parent's set with its assignment (sealed, wsnats.go) and keeps it as its
// base (.base/); its own files start as a copy. Whatever the child then
// edits is its branch: diff(base, own). When the parent's config changes,
// the child rebases - re-applies its diff onto the new base. When the child
// is stopped, its parent asks for that diff and merges it into its own
// config, saves it as a branch (branches/<name>.json - itself config, so it
// travels with everything else), or discards it.
//
// The diff/apply engine itself (per-value YAML tree diffing, named lists
// keyed by "name") is generic and lives in cfgtree/; this file is the
// Manager-bound orchestration on top of it - which files, which gateway,
// notifying children over NATS.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"d8z/gw/cfgfile"
	"d8z/gw/cfgtree"
	"d8z/gw/deployment"

	"gopkg.in/yaml.v3"
)

// cfgMu serializes reads and writes of the config dir and its base copy.
var cfgMu sync.Mutex

// cfgDir is this gateway's config dir (the one deployment.yaml is in).
func (m *Manager) cfgDir() string { return filepath.Dir(m.cfg.DeploymentFile) }

// baseDir holds the config our parent last gave us, to diff our own against.
func (m *Manager) baseDir() string { return filepath.Join(m.cfgDir(), ".base") }

// renderCfg renders tree t for file back to YAML, through the file's own
// type where we know it (so it keeps that type's field order and header).
func renderCfg(file string, t any) ([]byte, error) {
	switch file {
	case "workspace.yaml":
		return cfgtree.RenderAs(file, t, wsFileHeader, &wsFile{})
	case "deployment.yaml":
		return deployment.Render(t)
	}
	return yaml.Marshal(t)
}

// --- the child's side ---------------------------------------------------------

// configGift is what we hand a child: our config files, and the values of
// any env vars they name (token_env) - the child doesn't share our .env.
func (m *Manager) configGift() (map[string][]byte, map[string]string, error) {
	files, err := cfgtree.ReadFiles(m.cfgDir())
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{}
	for f, b := range files {
		if !cfgtree.IsTreeFile(f) {
			continue
		}
		t, err := cfgtree.ParseTree(b)
		if err != nil {
			continue
		}
		collectEnvRefs(t, env)
	}
	return files, env, nil
}

// collectEnvRefs walks a tree for "*token_env" keys and adds each named
// variable's value from our environment (when set) to env.
func collectEnvRefs(v any, env map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			if s, ok := e.(string); ok && strings.HasSuffix(k, "token_env") && s != "" {
				if val := os.Getenv(s); val != "" {
					env[s] = val
				}
			}
			collectEnvRefs(e, env)
		}
	case []any:
		for _, e := range t {
			collectEnvRefs(e, env)
		}
	}
}

// receiveConfig takes our parent's config: the first time, as our own (with
// the branch we were started from, if any, applied); after that by
// rebasing our own changes onto it.
func (m *Manager) receiveConfig(given map[string][]byte, env map[string]string, branch []cfgtree.Op) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	for k, v := range env {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	var ops []cfgtree.Op
	if _, err := os.Stat(m.baseDir()); err == nil {
		base, err := cfgtree.ReadFiles(m.baseDir())
		if err != nil {
			return err
		}
		own, err := cfgtree.ReadFiles(m.cfgDir())
		if err != nil {
			return err
		}
		if ops, err = cfgtree.Diff(base, own); err != nil {
			return err
		}
	} else {
		ops = branch
	}
	next, err := cfgtree.Apply(given, ops, renderCfg)
	if err != nil {
		return fmt.Errorf("rebasing onto the parent's config: %w", err)
	}
	if err := os.MkdirAll(m.baseDir(), 0o700); err != nil {
		return err
	}
	if err := cfgtree.WriteFiles(m.baseDir(), given); err != nil {
		return err
	}
	return cfgtree.WriteFiles(m.cfgDir(), next)
}

// configDiff is our branch: what we changed in what our parent gave us.
// Nil on a gateway with no parent (nothing was given).
func (m *Manager) configDiff() ([]cfgtree.Op, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if _, err := os.Stat(m.baseDir()); err != nil {
		return nil, nil
	}
	base, err := cfgtree.ReadFiles(m.baseDir())
	if err != nil {
		return nil, err
	}
	own, err := cfgtree.ReadFiles(m.cfgDir())
	if err != nil {
		return nil, err
	}
	return cfgtree.Diff(base, own)
}

// --- the parent's side --------------------------------------------------------

// mergeConfig applies a child's branch to our own config.
func (m *Manager) mergeConfig(ops []cfgtree.Op) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	own, err := cfgtree.ReadFiles(m.cfgDir())
	if err != nil {
		return err
	}
	next, err := cfgtree.Apply(own, ops, renderCfg)
	if err != nil {
		return err
	}
	if err := cfgtree.WriteFiles(m.cfgDir(), next); err != nil {
		return err
	}
	m.configChanged()
	return nil
}

// Branch is a child's config changes, saved when it was stopped without
// merging them: launch a child from it to pick up where that one left off.
type Branch struct {
	ID      string       `json:"id"`
	From    string       `json:"from"` // the gateway it came from: gw/<name> under ours
	Time    time.Time    `json:"time"`
	Ops     []cfgtree.Op `json:"ops"`
	Changes []string     `json:"changes"` // Ops, without values
}

// branchIDRE is what a branch id may look like (it becomes a file name).
var branchIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// branchFile is where branch id is saved.
func (m *Manager) branchFile(id string) string {
	return filepath.Join(m.cfgDir(), "branches", id+".json")
}

// saveBranch saves child from's config changes as a new branch, named
// "<from>-<UTC time>", and returns its id.
func (m *Manager) saveBranch(from string, ops []cfgtree.Op) (string, error) {
	now := time.Now().UTC()
	id := from + "-" + now.Format("20060102-150405")
	b := Branch{ID: id, From: "gw/" + from, Time: now, Ops: ops}
	for _, o := range ops {
		b.Changes = append(b.Changes, o.Summary())
	}
	raw, _ := json.MarshalIndent(b, "", "  ")
	if err := cfgfile.WriteFile(m.branchFile(id), raw); err != nil {
		return "", err
	}
	m.configChanged()
	return id, nil
}

// loadBranch reads saved branch id, values included.
func (m *Manager) loadBranch(id string) (*Branch, error) {
	if !branchIDRE.MatchString(id) {
		return nil, fmt.Errorf("bad branch id")
	}
	raw, err := os.ReadFile(m.branchFile(id))
	if err != nil {
		return nil, fmt.Errorf("no branch %q", id)
	}
	var b Branch
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// listBranches is every saved branch, newest first, without its Ops.
func (m *Manager) listBranches() []Branch {
	out := []Branch{}
	entries, _ := os.ReadDir(filepath.Join(m.cfgDir(), "branches"))
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		if b, err := m.loadBranch(id); err == nil {
			b.Ops = nil // values can be secrets
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

// deleteBranch removes saved branch id and tells our children.
func (m *Manager) deleteBranch(id string) error {
	if _, err := m.loadBranch(id); err != nil {
		return err
	}
	if err := os.Remove(m.branchFile(id)); err != nil {
		return err
	}
	m.configChanged()
	return nil
}

// cfgNotify records whether a configChanged notice is already scheduled.
var cfgNotify struct {
	sync.Mutex
	pending bool
}

// configChanged tells every running child our config changed, so it
// rebases (coalesced: a burst of writes is one notice).
func (m *Manager) configChanged() {
	if m.nats == nil {
		return
	}
	cfgNotify.Lock()
	if cfgNotify.pending {
		cfgNotify.Unlock()
		return
	}
	cfgNotify.pending = true
	cfgNotify.Unlock()
	time.AfterFunc(500*time.Millisecond, func() {
		cfgNotify.Lock()
		cfgNotify.pending = false
		cfgNotify.Unlock()
		m.notifyChildren()
	})
}
