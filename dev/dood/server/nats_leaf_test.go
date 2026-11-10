package main

// NATS leaf auth, exercised through a cloud target's own leaf credentials
// (the deployment package); the Deployment/Store logic itself has its own
// tests in that package.

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"d8z/gw/deployment"
)

// freePort is a localhost TCP port that was free a moment ago.
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
		NatsLeafUser: deployment.CloudLeafUser, NatsLeafPass: "s3cret"})
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
	root.setRemotes(map[string]string{"cloud:cloud1": "nats-leaf://" + deployment.CloudLeafUser + ":wrong@" + addr})
	time.Sleep(1500 * time.Millisecond)
	if root.LeafRemotes()[addr] {
		t.Fatal("leaf connected with a wrong password")
	}

	root.setRemotes(map[string]string{"cloud:cloud1": "nats-leaf://" + deployment.CloudLeafUser + ":s3cret@" + addr})
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
