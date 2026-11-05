package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A child's request lands in its own parent's queue - and only there: a
// grandchild's goes to its parent, never further up the tree.
func TestRequestsGoToTheParentOnly(t *testing.T) {
	lookupChildSpec = func(_ *Manager, _ context.Context, name string) (SpawnRequest, error) {
		return SpawnRequest{Name: name}, nil
	}
	defer func() { lookupChildSpec = (*Manager).childSpec }()

	hub := func(id string) (*NatsHub, int) {
		port := freePort(t)
		h, err := startNatsHub(Config{ID: id, NatsLeafPort: port, NatsStoreDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(h.Shutdown)
		return h, port
	}
	rootHub, _ := hub("root")
	wsHub, wsPort := hub("ws")
	grandHub, grandPort := hub("ws-rev")
	rootHub.setRemotes(map[string]string{"ws": fmt.Sprintf("nats-leaf://127.0.0.1:%d", wsPort)})
	wsHub.setRemotes(map[string]string{"rev": fmt.Sprintf("nats-leaf://127.0.0.1:%d", grandPort)})

	root := &Manager{cfg: Config{ID: "root", Role: "root"}, nats: rootHub}
	ws := &Manager{cfg: Config{ID: "ws", Role: "child"}, nats: wsHub}
	grand := &Manager{cfg: Config{ID: "ws-rev", Role: "child"}, nats: grandHub}
	root.serveRequests()
	ws.serveRequests()

	ask := func(m *Manager, reason string) {
		var err error
		for i := 0; i < 50; i++ {
			if err = m.AskParent("restart", reason); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal(err)
	}
	ask(grand, "new console")
	ask(ws, "new images")

	if r := ws.Requests(); len(r) != 1 || r[0].Gateway != "rev" || r[0].Reason != "new console" {
		t.Fatalf("ws queue: %+v", r)
	}
	if r := root.Requests(); len(r) != 1 || r[0].Gateway != "ws" || r[0].Reason != "new images" {
		t.Fatalf("root queue: %+v", r)
	}
	if err := root.AskParent("restart", ""); err == nil {
		t.Fatal("the root asked a parent it doesn't have")
	}
	if err := ws.queueRequest("rev", []byte(`{"kind":"delete-everything"}`)); err == nil {
		t.Fatal("queued an unknown kind")
	}
}
