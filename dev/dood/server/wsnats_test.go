package main

import (
	"path/filepath"
	"testing"
)

// TestSealRoundTripAndIsolation checks that each child gets its own stable
// key, that a sealed assignment opens with that key, and that a sibling's
// key can't open it.
func TestSealRoundTripAndIsolation(t *testing.T) {
	m := &Manager{cfg: Config{WorkspaceFile: filepath.Join(t.TempDir(), "workspace.yaml")}}
	ka, kb := m.childKey("wsa"), m.childKey("wsb")
	if ka == kb {
		t.Fatal("children must get different keys")
	}
	if m.childKey("wsa") != ka {
		t.Fatal("a child's key must be stable")
	}
	s, err := seal(ka, []byte(`{"token":"ghp_secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := unseal(ka, s); err != nil || string(got) != `{"token":"ghp_secret"}` {
		t.Fatalf("own key: %q %v", got, err)
	}
	if _, err := unseal(kb, s); err == nil {
		t.Fatal("a sibling's key must not unseal another workspace's assignment")
	}
}
