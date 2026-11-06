package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
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
	dir := t.TempDir()
	t.Setenv("DOCKER_HUB_USERNAME", "alice")
	t.Setenv("DOCKER_HUB_REPO", "stuff")
	m := &Manager{cfg: Config{DeploymentFile: filepath.Join(dir, "deployment.yaml"), WorkspaceFile: filepath.Join(dir, "workspace.yaml")}}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Docker Hub: token is write-only and survives an edit that leaves it blank.
	must(m.editDockerHub(hubInput{Username: "alice", Repository: "stuff", Token: "dckr_pat_x"}))
	must(m.editDockerHub(hubInput{Username: "alice", Repository: "things"}))
	d, _, err := m.loadDeployment()
	must(err)
	if d.DockerHub.Token != "dckr_pat_x" || d.ref("dood") != "alice/things:dood" {
		t.Fatalf("hub: %+v", d.DockerHub)
	}
	must(m.editDockerHub(hubInput{Username: "alice", Repository: "things", ClearToken: true, TokenEnv: "DOCKER_HUB_TOKEN"}))
	if d, _, _ = m.loadDeployment(); d.DockerHub.Token != "" {
		t.Fatal("token not cleared")
	}

	// Images: adding keeps the defaults; renaming a tag moves its targets.
	must(m.putImage("", HubImage{Name: "og", Service: "og-dh"}))
	must(m.putTarget("", CloudTarget{Name: "c1", Image: "dood", AllowFrom: []string{"1.2.3.4/32"}}))
	must(m.putImage("dood", HubImage{Name: "gw", Service: "dood-dh"}))
	d, _, _ = m.loadDeployment()
	if len(d.DockerHub.Images) != 3 {
		t.Fatalf("images: %+v", d.DockerHub.Images)
	}
	if tg, _ := d.target("c1"); tg.Image != "gw" {
		t.Fatalf("target didn't follow the rename: %+v", tg)
	}
	if err := m.removeImage("gw"); err == nil {
		t.Fatal("removed an image a target runs")
	}
	if err := m.putTarget("", CloudTarget{Name: "c2", Image: "nope"}); err == nil {
		t.Fatal("accepted a target with an unknown image")
	}

	// The file keeps only what was set: no defaults baked in.
	b, _ := os.ReadFile(m.cfg.DeploymentFile)
	if strings.Contains(string(b), "machine_type") || strings.Contains(string(b), "provider") {
		t.Fatalf("defaults written to the file:\n%s", b)
	}
	if st, _ := os.Stat(m.cfg.DeploymentFile); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}

	// GCP: a key is validated, stored beside the file, and reported by email.
	if err := m.editGCP(gcpInput{Project: "p", Zone: "us-west1-b", KeyJSON: `{"type":"user"}`}); err == nil {
		t.Fatal("accepted a non-service-account key")
	}
	if err := m.editGCP(gcpInput{Project: "p", Zone: "us-west1-b", Credentials: "deployment.yaml", KeyJSON: testKeyJSON(t)}); err == nil {
		t.Fatal("let a key overwrite deployment.yaml")
	}
	must(m.editGCP(gcpInput{Project: "p", Zone: "us-west1-b", KeyJSON: testKeyJSON(t)}))
	d, _, _ = m.loadDeployment()
	if m.gcpKeyEmail(d) != "deployer@proj.iam.gserviceaccount.com" {
		t.Fatal("key not stored")
	}
	must(m.editGCP(gcpInput{Project: "p", Zone: "us-west1-b", ClearKey: true}))
	if m.gcpKeyEmail(d) != "" {
		t.Fatal("key not cleared")
	}

	// A target with a VM can't be removed or renamed.
	must(m.setCloudState("c1", &cloudState{Instance: "d8z-c1", Status: "RUNNING"}))
	if err := m.removeTarget("c1"); err == nil {
		t.Fatal("removed a target that still has a VM")
	}
	if err := m.putTarget("c1", CloudTarget{Name: "c9", Image: "gw"}); err == nil {
		t.Fatal("renamed a target that still has a VM")
	}
	must(m.setCloudState("c1", nil))
	must(m.removeTarget("c1"))
}
