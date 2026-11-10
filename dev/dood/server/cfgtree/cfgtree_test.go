package cfgtree

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// render is a trivial render func for tests: no caller-specific types, just
// plain YAML back out.
func render(_ string, t any) ([]byte, error) { return yaml.Marshal(t) }

// roundTrip diffs base against own, applies the result to base, and
// returns the outcome (for comparing against own's parsed trees).
func roundTrip(t *testing.T, base, own map[string][]byte) map[string][]byte {
	t.Helper()
	ops, err := Diff(base, own)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	next, err := Apply(base, ops, render)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return next
}

// TestDiffApplyWholeFile checks a whole-file change (non-YAML) round-trips.
func TestDiffApplyWholeFile(t *testing.T) {
	base := map[string][]byte{"key.json": []byte(`{"a":1}`)}
	own := map[string][]byte{"key.json": []byte(`{"a":2}`)}
	next := roundTrip(t, base, own)
	if string(next["key.json"]) != string(own["key.json"]) {
		t.Errorf("key.json = %q, want %q", next["key.json"], own["key.json"])
	}
}

// TestDiffApplyRemoveWholeFile checks a file removed from own diffs and
// applies as a single remove op.
func TestDiffApplyRemoveWholeFile(t *testing.T) {
	base := map[string][]byte{"key.json": []byte(`{"a":1}`), "deployment.yaml": []byte("a: 1\n")}
	own := map[string][]byte{"deployment.yaml": []byte("a: 1\n")}
	ops, err := Diff(base, own)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(ops) != 1 || ops[0].File != "key.json" || ops[0].Op != "remove" {
		t.Fatalf("ops = %+v, want one remove of key.json", ops)
	}
	next, err := Apply(base, ops, render)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, ok := next["key.json"]; ok {
		t.Errorf("key.json still present after applying its remove")
	}
}

// TestDiffApplyMapValue checks a changed map field diffs to a single
// path-scoped set, and that Apply hands the caller's render func the right
// tree.
func TestDiffApplyMapValue(t *testing.T) {
	base := map[string][]byte{"deployment.yaml": []byte("docker_hub:\n  username: alice\n  repository: r\n")}
	own := map[string][]byte{"deployment.yaml": []byte("docker_hub:\n  username: bob\n  repository: r\n")}
	ops, err := Diff(base, own)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(ops) != 1 || ops[0].Op != "set" || len(ops[0].Path) != 2 || ops[0].Path[1] != "username" {
		t.Fatalf("ops = %+v, want one set at docker_hub.username", ops)
	}
	// Apply with a render that re-serializes the generic tree as plain YAML
	// (good enough to check the value landed, field order aside).
	next, err := Apply(base, ops, func(_ string, tree any) ([]byte, error) {
		m, ok := tree.(map[string]any)
		if !ok {
			t.Fatalf("tree = %#v, want a map", tree)
		}
		dh, ok := m["docker_hub"].(map[string]any)
		if !ok || dh["username"] != "bob" {
			t.Fatalf("docker_hub = %#v, want username bob", dh)
		}
		return []byte("rendered"), nil
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if string(next["deployment.yaml"]) != "rendered" {
		t.Errorf("deployment.yaml = %q, want %q", next["deployment.yaml"], "rendered")
	}
}

// TestDiffNamedList checks a named-list field diffs per entry (edit,
// remove, add) rather than as one whole-list change, and round-trips.
func TestDiffNamedList(t *testing.T) {
	base := map[string][]byte{"deployment.yaml": []byte("targets:\n  - name: a\n    disk_gb: 10\n  - name: b\n    disk_gb: 10\n")}
	own := map[string][]byte{"deployment.yaml": []byte("targets:\n  - name: a\n    disk_gb: 20\n  - name: c\n    disk_gb: 10\n")}
	ops, err := Diff(base, own)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	var sawEdit, sawRemoveB, sawAddC bool
	for _, o := range ops {
		switch {
		case o.Op == "set" && len(o.Path) == 3 && o.Path[1] == "[a]" && o.Path[2] == "disk_gb":
			sawEdit = true
		case o.Op == "remove" && len(o.Path) == 2 && o.Path[1] == "[b]":
			sawRemoveB = true
		case o.Op == "set" && len(o.Path) == 2 && o.Path[1] == "[c]":
			sawAddC = true
		}
	}
	if !sawEdit || !sawRemoveB || !sawAddC {
		t.Fatalf("ops = %+v, want an edit of [a], a remove of [b], an add of [c]", ops)
	}
	next := roundTrip(t, base, own)
	gotTree, err := ParseTree(next["deployment.yaml"])
	if err != nil {
		t.Fatalf("ParseTree(next): %v", err)
	}
	wantTree, err := ParseTree(own["deployment.yaml"])
	if err != nil {
		t.Fatalf("ParseTree(own): %v", err)
	}
	gotList, _ := namedList(gotTree.(map[string]any)["targets"])
	wantList, _ := namedList(wantTree.(map[string]any)["targets"])
	if !reflect.DeepEqual(gotList, wantList) {
		t.Errorf("roundtrip targets = %#v, want %#v", gotList, wantList)
	}
}

// TestApplyRejectsNonCfgFile checks Apply refuses an op naming a path
// IsCfgFile doesn't allow (e.g. one that escapes into .base/).
func TestApplyRejectsNonCfgFile(t *testing.T) {
	_, err := Apply(nil, []Op{{File: ".base/deployment.yaml", Op: "set", Raw: []byte("x")}}, render)
	if err == nil {
		t.Fatal("Apply: want an error for a non-config file path, got nil")
	}
}

// TestIsCfgFile checks which relative paths count as config that travels.
func TestIsCfgFile(t *testing.T) {
	cases := map[string]bool{
		"deployment.yaml":       true,
		"workspace.yaml":        true,
		".base/deployment.yaml": false,
		"deployment.state.json": false,
		"deployment.yaml.tmp":   false,
		"branches/a.json":       true,
		"branches/.hidden.json": false,
		"branches/sub/a.json":   false,
	}
	for rel, want := range cases {
		if got := IsCfgFile(rel); got != want {
			t.Errorf("IsCfgFile(%q) = %v, want %v", rel, got, want)
		}
	}
}
