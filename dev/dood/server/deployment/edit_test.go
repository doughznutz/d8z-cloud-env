package deployment

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"strings"
	"testing"
)

// testKeyJSON is a freshly generated, valid GCP service-account key file.
func testKeyJSON(t *testing.T) string {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "deployer@proj.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	return string(b)
}

// TestDeploymentEdits exercises the Deployment tab's edits: a write-only
// Docker Hub token, adding and renaming images, no defaults baked into the
// file, GCP key validation, and protecting targets that have a VM.
func TestDeploymentEdits(t *testing.T) {
	t.Setenv("DOCKER_HUB_USERNAME", "alice")
	t.Setenv("DOCKER_HUB_REPO", "stuff")
	s := testStore(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Docker Hub: token is write-only and survives an edit that leaves it blank.
	must(s.EditDockerHub(HubInput{Username: "alice", Repository: "stuff", Token: "dckr_pat_x"}))
	must(s.EditDockerHub(HubInput{Username: "alice", Repository: "things"}))
	d, _, err := s.Load()
	must(err)
	if d.DockerHub.Token != "dckr_pat_x" || d.Ref("dood") != "alice/things:dood" {
		t.Fatalf("hub: %+v", d.DockerHub)
	}
	must(s.EditDockerHub(HubInput{Username: "alice", Repository: "things", ClearToken: true, TokenEnv: "DOCKER_HUB_TOKEN"}))
	if d, _, _ = s.Load(); d.DockerHub.Token != "" {
		t.Fatal("token not cleared")
	}

	// Images: adding keeps the defaults; renaming a tag moves its targets.
	must(s.PutImage("", HubImage{Name: "og", Service: "og-dh"}))
	must(s.PutTarget("", CloudTarget{Name: "c1", Image: "dood", AllowFrom: []string{"1.2.3.4/32"}}))
	must(s.PutImage("dood", HubImage{Name: "gw", Service: "dood-dh"}))
	d, _, _ = s.Load()
	if len(d.DockerHub.Images) != 3 {
		t.Fatalf("images: %+v", d.DockerHub.Images)
	}
	if tg, _ := d.Target("c1"); tg.Image != "gw" {
		t.Fatalf("target didn't follow the rename: %+v", tg)
	}
	if err := s.RemoveImage("gw"); err == nil {
		t.Fatal("removed an image a target runs")
	}
	if err := s.PutTarget("", CloudTarget{Name: "c2", Image: "nope"}); err == nil {
		t.Fatal("accepted a target with an unknown image")
	}

	// The file keeps only what was set: no defaults baked in.
	b, _ := os.ReadFile(s.File())
	if strings.Contains(string(b), "machine_type") || strings.Contains(string(b), "provider") {
		t.Fatalf("defaults written to the file:\n%s", b)
	}
	if st, _ := os.Stat(s.File()); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}

	// GCP: a key is validated, stored beside the file, and reported by email.
	if err := s.EditGCP(GCPInput{Project: "p", Zone: "us-west1-b", KeyJSON: `{"type":"user"}`}); err == nil {
		t.Fatal("accepted a non-service-account key")
	}
	if err := s.EditGCP(GCPInput{Project: "p", Zone: "us-west1-b", Credentials: "deployment.yaml", KeyJSON: testKeyJSON(t)}); err == nil {
		t.Fatal("let a key overwrite deployment.yaml")
	}
	must(s.EditGCP(GCPInput{Project: "p", Zone: "us-west1-b", KeyJSON: testKeyJSON(t)}))
	d, _, _ = s.Load()
	if s.GCPKeyEmail(d) != "deployer@proj.iam.gserviceaccount.com" {
		t.Fatal("key not stored")
	}
	must(s.EditGCP(GCPInput{Project: "p", Zone: "us-west1-b", ClearKey: true}))
	if s.GCPKeyEmail(d) != "" {
		t.Fatal("key not cleared")
	}

	// A target with a VM can't be removed or renamed.
	must(s.SetState("c1", &CloudState{Instance: "d8z-c1", Status: "RUNNING"}))
	if err := s.RemoveTarget("c1"); err == nil {
		t.Fatal("removed a target that still has a VM")
	}
	if err := s.PutTarget("c1", CloudTarget{Name: "c9", Image: "gw"}); err == nil {
		t.Fatal("renamed a target that still has a VM")
	}
	must(s.SetState("c1", nil))
	must(s.RemoveTarget("c1"))
}
