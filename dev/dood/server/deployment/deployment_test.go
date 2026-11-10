package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

// testStore is a Store whose deployment.yaml is a fresh temp file, with no
// NATS and no leaf-password deriver (neither is exercised by these tests).
func testStore(t *testing.T) *Store {
	dir := t.TempDir()
	return NewStore(filepath.Join(dir, "deployment.yaml"), "", "", nil, nil)
}

// TestDeploymentDefaultsAndValidation checks that a missing or sparse
// deployment.yaml is filled in from .env and defaults, and that invalid
// targets and credentials paths are rejected.
func TestDeploymentDefaultsAndValidation(t *testing.T) {
	t.Setenv("DOCKER_HUB_USERNAME", "alice")
	t.Setenv("DOCKER_HUB_REPO", "stuff")
	s := testStore(t)

	d, fromFile, err := s.Load()
	if err != nil || fromFile {
		t.Fatalf("no file: %v %v", fromFile, err)
	}
	if d.Ref("dood") != "alice/stuff:dood" || d.Ref("d8z") != "alice/stuff:d8z" || len(d.DockerHub.Images) != 2 {
		t.Fatalf("defaults: %+v", d.DockerHub)
	}

	write := func(v string) {
		if err := os.WriteFile(s.File(), []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("targets:\n  - name: cloud1\n    allow_from: [203.0.113.7/32]\n")
	d, _, err = s.Load()
	if err != nil {
		t.Fatal(err)
	}
	tg, _ := d.Target("cloud1")
	if tg.Profile != "root" || tg.Image != "dood" || tg.Provider != "gcp" || tg.MachineType == "" {
		t.Fatalf("target defaults: %+v", tg)
	}

	for _, bad := range []string{
		"targets:\n  - name: Bad-Name\n",
		"targets:\n  - name: c1\n    image: nope\n",
		"targets:\n  - name: c1\n    allow_from: [1.2.3.4]\n",
		"targets:\n  - name: c1\n    profile: everything\n",
		"gcp:\n  credentials: ../key.json\n",
	} {
		write(bad)
		if _, _, err := s.Load(); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
