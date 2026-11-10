// Package cfgtree is the generic YAML-tree diff/apply engine behind a
// gateway's config branch/merge, branch/merge style:
//
// A gateway's config is the set of files in its config dir (workspace.yaml,
// deployment.yaml, key files, saved branches/). A child is given its
// parent's set with its assignment (sealed) and keeps it as its base
// (.base/); its own files start as a copy. Whatever the child then edits is
// its branch: Diff(base, own). When the parent's config changes, the child
// rebases - re-applies its diff onto the new base. When the child is
// stopped, its parent asks for that diff and merges it into its own
// config, saves it as a branch, or discards it. All of that orchestration
// (which files, which Manager, NATS) lives with its caller; this package
// only knows about bytes, trees and Ops.
//
// Diffs are per value, not per line: YAML files are compared as trees, and a
// list of objects with a "name" is keyed by name (so a workspace, image or
// target is one entry however the list is ordered). Any other file is
// compared whole.
package cfgtree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"d8z/gw/cfgfile"

	"gopkg.in/yaml.v3"
)

// Op is one change: set or remove a value at Path in File. An empty Path is
// the whole file (Value is its parsed tree, or Raw its bytes).
type Op struct {
	File  string   `json:"file"`
	Path  []string `json:"path,omitempty"`  // map keys, or "[name]" for a named list entry
	Op    string   `json:"op"`              // set | remove
	Value any      `json:"value,omitempty"` // the new value, for a set in a YAML file
	Raw   []byte   `json:"raw,omitempty"`   // the new bytes, for a set of a whole non-YAML file
}

// Summary is the op without its value (values can be secrets).
func (o Op) Summary() string {
	p := o.File
	if len(o.Path) > 0 {
		p += ": " + strings.Join(o.Path, ".")
	}
	return o.Op + " " + p
}

// IsCfgFile says whether a path (relative to a config dir) is config that
// travels: top-level files except dotfiles and runtime state, plus saved
// branches.
func IsCfgFile(rel string) bool {
	if dir, f := filepath.Split(rel); dir == "branches/" {
		return strings.HasSuffix(f, ".json") && !strings.HasPrefix(f, ".")
	} else if dir != "" {
		return false
	}
	return !strings.HasPrefix(rel, ".") && !strings.HasSuffix(rel, ".state.json") && !strings.HasSuffix(rel, ".tmp")
}

// ReadFiles reads every config file in dir (see IsCfgFile), keyed by
// slash-separated path relative to dir. A missing dir is an empty set.
func ReadFiles(dir string) (map[string][]byte, error) {
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
			if !e.Type().IsRegular() || !IsCfgFile(rel) {
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

// WriteFiles makes dir's config files exactly files.
func WriteFiles(dir string, files map[string][]byte) error {
	old, err := ReadFiles(dir)
	if err != nil {
		return err
	}
	for rel := range old {
		if _, ok := files[rel]; !ok {
			os.Remove(filepath.Join(dir, rel))
		}
	}
	for rel, b := range files {
		if !IsCfgFile(rel) {
			return fmt.Errorf("not a config file: %s", rel)
		}
		if bytes.Equal(old[rel], b) {
			continue
		}
		if err := cfgfile.WriteFile(filepath.Join(dir, rel), b); err != nil {
			return err
		}
	}
	return nil
}

// IsTreeFile says whether a config file is YAML, and so diffed per value
// rather than whole.
func IsTreeFile(rel string) bool {
	return strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml")
}

// ParseTree parses YAML into generic maps, lists and scalars.
func ParseTree(b []byte) (any, error) {
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// RenderAs renders tree t back to YAML through v (a pointer to the file's
// own type, so the result keeps that type's field order), with header
// written first. file names it for error context only.
func RenderAs(file string, t any, header string, v any) ([]byte, error) {
	raw, err := yaml.Marshal(t)
	if err != nil {
		return nil, err
	}
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

// --- diff -------------------------------------------------------------------

// Diff is the ops that turn the base file set into own, in file order: a
// removed file is one remove, a non-YAML or new file one whole-file set,
// and a changed YAML file is diffed per value (diffNode).
func Diff(base, own map[string][]byte) ([]Op, error) {
	var ops []Op
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
			ops = append(ops, Op{File: f, Op: "remove"})
		case bytes.Equal(a, b):
		case !IsTreeFile(f):
			ops = append(ops, Op{File: f, Op: "set", Raw: b})
		case !inA:
			t, err := ParseTree(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			ops = append(ops, Op{File: f, Op: "set", Value: t})
		default:
			ta, err := ParseTree(a)
			if err != nil {
				return nil, fmt.Errorf("%s (base): %w", f, err)
			}
			tb, err := ParseTree(b)
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
func diffNode(ops []Op, file string, path []string, a, b any) []Op {
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
					ops = append(ops, Op{File: file, Path: at(k), Op: "remove"})
				case !inA:
					ops = append(ops, Op{File: file, Path: at(k), Op: "set", Value: vb})
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
					ops = append(ops, Op{File: file, Path: at(seg), Op: "remove"})
				case !inA:
					ops = append(ops, Op{File: file, Path: at(seg), Op: "set", Value: vb})
				default:
					ops = diffNode(ops, file, at(seg), va, vb)
				}
			}
			return ops
		}
	}
	if !reflect.DeepEqual(a, b) {
		ops = append(ops, Op{File: file, Path: path, Op: "set", Value: b})
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

// Apply applies ops to a copy of files. A change to something the new base
// no longer has is applied anyway (a set creates it, a remove of something
// gone is a no-op): the branch's intent wins. render renders a file's tree
// back to bytes - the caller knows each file's own type (for field order
// and header), this package only knows generic trees.
func Apply(files map[string][]byte, ops []Op, render func(file string, tree any) ([]byte, error)) (map[string][]byte, error) {
	out := map[string][]byte{}
	for k, v := range files {
		out[k] = v
	}
	trees := map[string]any{}
	for _, o := range ops {
		if !IsCfgFile(o.File) {
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
				if t, err = ParseTree(b); err != nil {
					return nil, fmt.Errorf("%s: %w", o.File, err)
				}
			}
		}
		t, err := applyAt(t, o.Path, o.Op, o.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.Summary(), err)
		}
		trees[o.File] = t
	}
	for f, t := range trees {
		b, err := render(f, t)
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
