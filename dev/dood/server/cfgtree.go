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
// Diffs are per value, not per line: YAML files are compared as trees, and a
// list of objects with a "name" is keyed by name (so a workspace, image or
// target is one entry however the list is ordered). Any other file is
// compared whole.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// cfgOp is one change: set or remove a value at Path in File. An empty Path
// is the whole file (Value is its parsed tree, or Raw its bytes).
type cfgOp struct {
	File  string   `json:"file"`
	Path  []string `json:"path,omitempty"`  // map keys, or "[name]" for a named list entry
	Op    string   `json:"op"`              // set | remove
	Value any      `json:"value,omitempty"` // the new value, for a set in a YAML file
	Raw   []byte   `json:"raw,omitempty"`   // the new bytes, for a set of a whole non-YAML file
}

// summary is the op without its value (values can be secrets).
func (o cfgOp) summary() string {
	p := o.File
	if len(o.Path) > 0 {
		p += ": " + strings.Join(o.Path, ".")
	}
	return o.Op + " " + p
}

// cfgMu serializes reads and writes of the config dir and its base copy.
var cfgMu sync.Mutex

// cfgDir is this gateway's config dir (the one deployment.yaml is in).
func (m *Manager) cfgDir() string { return filepath.Dir(m.cfg.DeploymentFile) }

// baseDir holds the config our parent last gave us, to diff our own against.
func (m *Manager) baseDir() string { return filepath.Join(m.cfgDir(), ".base") }

// isCfgFile says whether a path (relative to a config dir) is config that
// travels: top-level files except dotfiles and runtime state, plus saved
// branches.
func isCfgFile(rel string) bool {
	if dir, f := filepath.Split(rel); dir == "branches/" {
		return strings.HasSuffix(f, ".json") && !strings.HasPrefix(f, ".")
	} else if dir != "" {
		return false
	}
	return !strings.HasPrefix(rel, ".") && !strings.HasSuffix(rel, ".state.json") && !strings.HasSuffix(rel, ".tmp")
}

// readCfgFiles reads every config file in dir (see isCfgFile), keyed by
// slash-separated path relative to dir. A missing dir is an empty set.
func readCfgFiles(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, sub := range []string{"", "branches"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			rel := filepath.ToSlash(filepath.Join(sub, e.Name()))
			if !e.Type().IsRegular() || !isCfgFile(rel) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				return nil, err
			}
			out[rel] = b
		}
	}
	return out, nil
}

// writeCfgFiles makes dir's config files exactly files.
func writeCfgFiles(dir string, files map[string][]byte) error {
	old, err := readCfgFiles(dir)
	if err != nil {
		return err
	}
	for rel := range old {
		if _, ok := files[rel]; !ok {
			os.Remove(filepath.Join(dir, rel))
		}
	}
	for rel, b := range files {
		if !isCfgFile(rel) {
			return fmt.Errorf("not a config file: %s", rel)
		}
		if bytes.Equal(old[rel], b) {
			continue
		}
		if err := writeConfigFile(filepath.Join(dir, rel), b); err != nil {
			return err
		}
	}
	return nil
}

// isTreeFile says whether a config file is YAML, and so diffed per value
// rather than whole.
func isTreeFile(rel string) bool {
	return strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml")
}

// parseTree parses YAML into generic maps, lists and scalars.
func parseTree(b []byte) (any, error) {
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// --- diff -------------------------------------------------------------------

// diffCfg is the ops that turn the base file set into own, in file order:
// a removed file is one remove, a non-YAML or new file one whole-file set,
// and a changed YAML file is diffed per value (diffNode).
func diffCfg(base, own map[string][]byte) ([]cfgOp, error) {
	var ops []cfgOp
	names := map[string]bool{}
	for k := range base {
		names[k] = true
	}
	for k := range own {
		names[k] = true
	}
	for _, f := range sortedKeys(names) {
		a, inA := base[f]
		b, inB := own[f]
		switch {
		case !inB:
			ops = append(ops, cfgOp{File: f, Op: "remove"})
		case bytes.Equal(a, b):
		case !isTreeFile(f):
			ops = append(ops, cfgOp{File: f, Op: "set", Raw: b})
		case !inA:
			t, err := parseTree(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			ops = append(ops, cfgOp{File: f, Op: "set", Value: t})
		default:
			ta, err := parseTree(a)
			if err != nil {
				return nil, fmt.Errorf("%s (base): %w", f, err)
			}
			tb, err := parseTree(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			ops = diffNode(ops, f, nil, ta, tb)
		}
	}
	return ops, nil
}

// diffNode appends to ops the changes from tree a to tree b at path in
// file. Maps and named lists are recursed into key by key; anything else
// that differs is one set of b's value.
func diffNode(ops []cfgOp, file string, path []string, a, b any) []cfgOp {
	at := func(k string) []string { return append(append([]string{}, path...), k) }
	if ma, ok := a.(map[string]any); ok {
		if mb, ok := b.(map[string]any); ok {
			keys := map[string]bool{}
			for k := range ma {
				keys[k] = true
			}
			for k := range mb {
				keys[k] = true
			}
			for _, k := range sortedKeys(keys) {
				va, inA := ma[k]
				vb, inB := mb[k]
				switch {
				case !inB:
					ops = append(ops, cfgOp{File: file, Path: at(k), Op: "remove"})
				case !inA:
					ops = append(ops, cfgOp{File: file, Path: at(k), Op: "set", Value: vb})
				default:
					ops = diffNode(ops, file, at(k), va, vb)
				}
			}
			return ops
		}
	}
	if na, ok := namedList(a); ok {
		if nb, ok := namedList(b); ok {
			keys := map[string]bool{}
			for k := range na {
				keys[k] = true
			}
			for k := range nb {
				keys[k] = true
			}
			for _, k := range sortedKeys(keys) {
				va, inA := na[k]
				vb, inB := nb[k]
				seg := "[" + k + "]"
				switch {
				case !inB:
					ops = append(ops, cfgOp{File: file, Path: at(seg), Op: "remove"})
				case !inA:
					ops = append(ops, cfgOp{File: file, Path: at(seg), Op: "set", Value: vb})
				default:
					ops = diffNode(ops, file, at(seg), va, vb)
				}
			}
			return ops
		}
	}
	if !reflect.DeepEqual(a, b) {
		ops = append(ops, cfgOp{File: file, Path: path, Op: "set", Value: b})
	}
	return ops
}

// namedList is a list whose every entry is an object with a unique string
// "name", keyed by it. (An empty list counts: it can grow named entries.)
func namedList(v any) (map[string]any, bool) {
	l, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := map[string]any{}
	for _, e := range l {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, false
		}
		n, ok := m["name"].(string)
		if !ok || n == "" {
			return nil, false
		}
		if _, dup := out[n]; dup {
			return nil, false
		}
		out[n] = e
	}
	return out, true
}

// sortedKeys is m's keys in sorted order, so diffs are deterministic.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- apply ------------------------------------------------------------------

// applyCfg applies ops to a copy of files. A change to something the new
// base no longer has is applied anyway (a set creates it, a remove of
// something gone is a no-op): the branch's intent wins.
func applyCfg(files map[string][]byte, ops []cfgOp) (map[string][]byte, error) {
	out := map[string][]byte{}
	for k, v := range files {
		out[k] = v
	}
	trees := map[string]any{}
	for _, o := range ops {
		if !isCfgFile(o.File) {
			return nil, fmt.Errorf("not a config file: %s", o.File)
		}
		if len(o.Path) == 0 {
			delete(trees, o.File)
			switch {
			case o.Op == "remove":
				delete(out, o.File)
			case o.Raw != nil:
				out[o.File] = o.Raw
			default:
				trees[o.File] = o.Value
			}
			continue
		}
		t, ok := trees[o.File]
		if !ok {
			if b, ok := out[o.File]; ok {
				var err error
				if t, err = parseTree(b); err != nil {
					return nil, fmt.Errorf("%s: %w", o.File, err)
				}
			}
		}
		t, err := applyAt(t, o.Path, o.Op, o.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.summary(), err)
		}
		trees[o.File] = t
	}
	for f, t := range trees {
		b, err := renderCfgTree(f, t)
		if err != nil {
			return nil, err
		}
		out[f] = b
	}
	return out, nil
}

// applyAt returns a copy of node with op applied at path. A "[name]"
// segment addresses a named list entry, created if missing on a set; any
// other segment is a map key. Inputs are never mutated.
func applyAt(node any, path []string, op string, val any) (any, error) {
	if len(path) == 0 {
		if op == "remove" {
			return nil, nil
		}
		return val, nil
	}
	seg, rest := path[0], path[1:]
	if strings.HasPrefix(seg, "[") && strings.HasSuffix(seg, "]") {
		name := seg[1 : len(seg)-1]
		l, _ := node.([]any)
		if node != nil && l == nil {
			return nil, fmt.Errorf("%s: not a list", seg)
		}
		for i, e := range l {
			if m, ok := e.(map[string]any); ok && m["name"] == name {
				if op == "remove" && len(rest) == 0 {
					return append(append([]any{}, l[:i]...), l[i+1:]...), nil
				}
				nv, err := applyAt(e, rest, op, val)
				if err != nil {
					return nil, err
				}
				l = append([]any{}, l...)
				l[i] = nv
				return l, nil
			}
		}
		if op == "remove" {
			return node, nil
		}
		nv, err := applyAt(map[string]any{"name": name}, rest, op, val)
		if err != nil {
			return nil, err
		}
		return append(append([]any{}, l...), nv), nil
	}
	m, _ := node.(map[string]any)
	if node != nil && m == nil {
		return nil, fmt.Errorf("%s: not a map", seg)
	}
	cp := map[string]any{}
	for k, v := range m {
		cp[k] = v
	}
	if op == "remove" && len(rest) == 0 {
		delete(cp, seg)
		return cp, nil
	}
	nv, err := applyAt(cp[seg], rest, op, val)
	if err != nil {
		return nil, err
	}
	cp[seg] = nv
	return cp, nil
}

// renderCfgTree writes a tree back out - the files we know through their
// own types, so they keep their field order and header.
func renderCfgTree(file string, t any) ([]byte, error) {
	raw, err := yaml.Marshal(t)
	if err != nil {
		return nil, err
	}
	enc := func(header string, v any) ([]byte, error) {
		if err := yaml.Unmarshal(raw, v); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		var buf bytes.Buffer
		e := yaml.NewEncoder(&buf)
		e.SetIndent(2)
		if err := e.Encode(v); err != nil {
			return nil, err
		}
		return append([]byte(header+"\n"), buf.Bytes()...), nil
	}
	switch file {
	case "workspace.yaml":
		return enc(wsFileHeader, &wsFile{})
	case "deployment.yaml":
		return enc(deploymentFileHeader, &Deployment{})
	}
	return raw, nil
}

// --- the child's side ---------------------------------------------------------

// configGift is what we hand a child: our config files, and the values of
// any env vars they name (token_env) - the child doesn't share our .env.
func (m *Manager) configGift() (map[string][]byte, map[string]string, error) {
	files, err := readCfgFiles(m.cfgDir())
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{}
	for f, b := range files {
		if !isTreeFile(f) {
			continue
		}
		t, err := parseTree(b)
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
func (m *Manager) receiveConfig(given map[string][]byte, env map[string]string, branch []cfgOp) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	for k, v := range env {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	var ops []cfgOp
	if _, err := os.Stat(m.baseDir()); err == nil {
		base, err := readCfgFiles(m.baseDir())
		if err != nil {
			return err
		}
		own, err := readCfgFiles(m.cfgDir())
		if err != nil {
			return err
		}
		if ops, err = diffCfg(base, own); err != nil {
			return err
		}
	} else {
		ops = branch
	}
	next, err := applyCfg(given, ops)
	if err != nil {
		return fmt.Errorf("rebasing onto the parent's config: %w", err)
	}
	if err := os.MkdirAll(m.baseDir(), 0o700); err != nil {
		return err
	}
	if err := writeCfgFiles(m.baseDir(), given); err != nil {
		return err
	}
	return writeCfgFiles(m.cfgDir(), next)
}

// configDiff is our branch: what we changed in what our parent gave us.
// Nil on a gateway with no parent (nothing was given).
func (m *Manager) configDiff() ([]cfgOp, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if _, err := os.Stat(m.baseDir()); err != nil {
		return nil, nil
	}
	base, err := readCfgFiles(m.baseDir())
	if err != nil {
		return nil, err
	}
	own, err := readCfgFiles(m.cfgDir())
	if err != nil {
		return nil, err
	}
	return diffCfg(base, own)
}

// --- the parent's side --------------------------------------------------------

// mergeConfig applies a child's branch to our own config.
func (m *Manager) mergeConfig(ops []cfgOp) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	own, err := readCfgFiles(m.cfgDir())
	if err != nil {
		return err
	}
	next, err := applyCfg(own, ops)
	if err != nil {
		return err
	}
	if err := writeCfgFiles(m.cfgDir(), next); err != nil {
		return err
	}
	m.configChanged()
	return nil
}

// Branch is a child's config changes, saved when it was stopped without
// merging them: launch a child from it to pick up where that one left off.
type Branch struct {
	ID      string    `json:"id"`
	From    string    `json:"from"` // the gateway it came from: gw/<name> under ours
	Time    time.Time `json:"time"`
	Ops     []cfgOp   `json:"ops"`
	Changes []string  `json:"changes"` // Ops, without values
}

// branchIDRE is what a branch id may look like (it becomes a file name).
var branchIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// branchFile is where branch id is saved.
func (m *Manager) branchFile(id string) string {
	return filepath.Join(m.cfgDir(), "branches", id+".json")
}

// saveBranch saves child from's config changes as a new branch, named
// "<from>-<UTC time>", and returns its id.
func (m *Manager) saveBranch(from string, ops []cfgOp) (string, error) {
	now := time.Now().UTC()
	id := from + "-" + now.Format("20060102-150405")
	b := Branch{ID: id, From: "gw/" + from, Time: now, Ops: ops}
	for _, o := range ops {
		b.Changes = append(b.Changes, o.summary())
	}
	raw, _ := json.MarshalIndent(b, "", "  ")
	if err := writeConfigFile(m.branchFile(id), raw); err != nil {
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
