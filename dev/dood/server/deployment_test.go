package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// A cloud gateway's leaf listener requires credentials: the root dials it
// with them, then a message and a ping cross the link - and a remote with a
// wrong password never gets a link.
func TestCloudLeafAuthAndPing(t *testing.T) {
	port := freePort(t)
	cloud, err := startNatsHub(Config{ID: "cloud1", NatsLeafPort: port, NatsStoreDir: t.TempDir(),
		NatsLeafUser: cloudLeafUser, NatsLeafPass: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	defer cloud.Shutdown()
	cloud.ServePing()

	root, err := startNatsHub(Config{ID: "root", NatsLeafPort: freePort(t), NatsStoreDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Shutdown()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	root.setRemotes(map[string]string{"cloud:cloud1": "nats-leaf://" + cloudLeafUser + ":wrong@" + addr})
	time.Sleep(1500 * time.Millisecond)
	if root.LeafRemotes()[addr] {
		t.Fatal("leaf connected with a wrong password")
	}

	root.setRemotes(map[string]string{"cloud:cloud1": "nats-leaf://" + cloudLeafUser + ":s3cret@" + addr})
	var reply string
	for i := 0; i < 50; i++ {
		if reply, err = root.Ping("cloud1", "hi", 200*time.Millisecond); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("ping over the leaf: %v", err)
	}
	if !strings.Contains(reply, `"id":"cloud1"`) || !strings.Contains(reply, `"got":"hi"`) {
		t.Fatalf("unexpected reply %s", reply)
	}
	if !root.LeafRemotes()[addr] {
		t.Fatal("LeafRemotes doesn't report the connected link")
	}

	if err := root.Publish("cloud1", "hello cloud"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		msgs, _ := cloud.Inbox(10)
		for _, m := range msgs {
			if m.Subject == "gw.cloud1.inbox" && m.Data == "hello cloud" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("message never reached the cloud gateway's inbox")
}

func TestDeploymentDefaultsAndValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_HUB_USERNAME", "alice")
	t.Setenv("DOCKER_HUB_REPO", "stuff")
	m := &Manager{cfg: Config{DeploymentFile: filepath.Join(dir, "deployment.yaml")}}

	d, fromFile, err := m.loadDeployment()
	if err != nil || fromFile {
		t.Fatalf("no file: %v %v", fromFile, err)
	}
	if d.ref("dood") != "alice/stuff:dood" || d.ref("d8z") != "alice/stuff:d8z" || len(d.DockerHub.Images) != 2 {
		t.Fatalf("defaults: %+v", d.DockerHub)
	}

	write := func(s string) {
		if err := os.WriteFile(m.cfg.DeploymentFile, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("targets:\n  - name: cloud1\n    allow_from: [203.0.113.7/32]\n")
	d, _, err = m.loadDeployment()
	if err != nil {
		t.Fatal(err)
	}
	tg, _ := d.target("cloud1")
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
		if _, _, err := m.loadDeployment(); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
