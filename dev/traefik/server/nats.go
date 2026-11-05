package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// Every gateway in the cascade embeds its own NATS server, forming a tree of
// leaf node connections that mirrors the Traefik/gwlink cascade exactly:
// a running child listens for leaf connections on NatsLeafPort, and its
// parent - never the child - dials in once it has attached to the child's
// gwlink network. That's the same direction (and the same trigger, right
// after Manager.attach) as the existing HTTP upstream routing in
// Reconcile/renderRoute, so no address needs to flow the other way and no
// child ever needs to discover its parent.
//
// Subjects are addressed by GW_ID: names can't contain "-" or ".", so
// dash-joined IDs ("abc-xyz") convert to dotted subject tokens ("abc.xyz")
// unambiguously. Each node keeps its own local JetStream stream for
// "gw.<dotted-id>.>" so its own messages survive a process restart; live
// visibility into a whole subtree comes for free from leaf node subject
// propagation, with no need to replicate streams across leaf boundaries.
type NatsHub struct {
	id   string
	opts *server.Options
	srv  *server.Server
	nc   *nats.Conn
	js   nats.JetStreamContext
}

// natsSubjectID is the dotted token for a GW_ID.
func natsSubjectID(id string) string { return strings.ReplaceAll(id, "-", ".") }

// gwSubject is one of this (or another) gateway's subjects, e.g.
// gwSubject("abc-xyz", "health") -> "gw.abc.xyz.health".
func gwSubject(id, suffix string) string {
	return "gw." + natsSubjectID(id) + "." + suffix
}

// startNatsHub brings up the embedded server, its JetStream store, and an
// in-process client (no TCP client port - nothing outside this process
// needs one; only the leafnode port is reachable from other containers).
func startNatsHub(cfg Config) (*NatsHub, error) {
	if err := os.MkdirAll(cfg.NatsStoreDir, 0o755); err != nil {
		return nil, fmt.Errorf("nats store dir: %w", err)
	}
	opts := &server.Options{
		ServerName: "gw-" + cfg.ID,
		DontListen: true, // no plain client listener; see NatsHub.nc below
		LeafNode: server.LeafNodeOpts{
			Host: "0.0.0.0", // only reachable over gwlink, which is internal-only
			Port: cfg.NatsLeafPort,
		},
		JetStream: true,
		StoreDir:  cfg.NatsStoreDir,
		NoLog:     true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("nats server: %w", err)
	}
	srv.ConfigureLogger()
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		return nil, fmt.Errorf("nats server didn't become ready")
	}

	nc, err := nats.Connect("", nats.InProcessServer(srv))
	if err != nil {
		srv.Shutdown()
		return nil, fmt.Errorf("nats in-process connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		srv.Shutdown()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}

	h := &NatsHub{id: cfg.ID, opts: opts, srv: srv, nc: nc, js: js}
	if err := h.ensureStream(); err != nil {
		log.Printf("nats: stream: %v", err)
	}
	return h, nil
}

// streamName is this GW_ID's own JetStream stream name - names can't
// contain "." (or "-", already excluded by the gateway name regex), so
// underscores keep it a valid stream name while staying recognizable.
func streamName(id string) string { return "gw_" + strings.ReplaceAll(id, "-", "_") }

// ensureStream creates this node's own stream if it isn't already there
// (e.g. recovered from the JetStream store on a process restart).
func (h *NatsHub) ensureStream() error {
	name := streamName(h.id)
	if _, err := h.js.StreamInfo(name); err == nil {
		return nil
	}
	_, err := h.js.AddStream(&nats.StreamConfig{
		Name:     name,
		Subjects: []string{gwSubject(h.id, ">")},
	})
	return err
}

// Publish sends a plain message into id's own inbox subject. A plain
// core-NATS publish - not the acked JetStream API - is what's proven to
// cross a leaf link on its own (see nats_test.go's cross-leaf case): it's
// captured by whichever node's local stream owns that id, local or several
// leaf hops away, with no JetStream account/domain boundary involved.
func (h *NatsHub) Publish(id, text string) error {
	if err := h.nc.Publish(gwSubject(id, "inbox"), []byte(text)); err != nil {
		return err
	}
	// These are low-volume control-plane sends where an HTTP caller may
	// immediately read the inbox back; nc.Publish only buffers client-side,
	// so flush rather than leave delivery to the client's own timer.
	return h.nc.FlushTimeout(2 * time.Second)
}

// Hello publishes a one-off message to this gateway's own hello subject -
// the simplest possible proof that the embedded server, JetStream, and the
// in-process client are all actually working.
func (h *NatsHub) Hello(text string) error {
	if err := h.nc.Publish(gwSubject(h.id, "hello"), []byte(text)); err != nil {
		return err
	}
	return h.nc.FlushTimeout(2 * time.Second)
}

// StartHealthLoop publishes a heartbeat on this gateway's own health
// subject every interval, until stop is closed.
func (h *NatsHub) StartHealthLoop(interval time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				payload := fmt.Sprintf(`{"id":%q,"ts":%q,"status":"ok"}`, h.id, now.UTC().Format(time.RFC3339))
				if err := h.nc.Publish(gwSubject(h.id, "health"), []byte(payload)); err != nil {
					log.Printf("nats: health publish: %v", err)
				}
			}
		}
	}()
}

// InboxMsg is one message read back from this gateway's own stream.
type InboxMsg struct {
	Seq     uint64    `json:"seq"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
	Data    string    `json:"data"`
}

// Inbox returns up to max of this gateway's own recent messages (any
// gw.<this-id>.* subject - hello, health, inbox, ...), oldest first. Always
// local/self: a console sees a descendant's inbox by talking to that
// descendant's own gw-server, the same way it already reaches any other
// per-gateway data - see Manager.probe and the console's iframes.
func (h *NatsHub) Inbox(max int) ([]InboxMsg, error) {
	if max <= 0 || max > 500 {
		max = 50
	}
	name := streamName(h.id)
	info, err := h.js.StreamInfo(name)
	if err != nil {
		return nil, err
	}
	out := make([]InboxMsg, 0, max)
	if info.State.Msgs == 0 {
		return out, nil
	}
	start := info.State.FirstSeq
	if info.State.LastSeq-start+1 > uint64(max) {
		start = info.State.LastSeq - uint64(max) + 1
	}
	for seq := start; seq <= info.State.LastSeq; seq++ {
		raw, err := h.js.GetMsg(name, seq)
		if err != nil {
			continue // purged/deleted sequence
		}
		out = append(out, InboxMsg{Seq: raw.Sequence, Subject: raw.Subject, Time: raw.Time, Data: string(raw.Data)})
	}
	return out, nil
}

// setChildRemotes reconciles this hub's leaf node remotes against the given
// set of running children (name -> container name), dialing into each
// child's leaf listener. Only called by the parent; a child never has
// remotes of its own pointed at anything (it just listens).
func (h *NatsHub) setChildRemotes(leafPort int, children map[string]string) {
	remotes := make([]*server.RemoteLeafOpts, 0, len(children))
	for _, container := range children {
		u, err := url.Parse("nats-leaf://" + container + ":" + strconv.Itoa(leafPort))
		if err != nil {
			continue
		}
		remotes = append(remotes, &server.RemoteLeafOpts{URLs: []*url.URL{u}})
	}
	if sameRemotes(h.opts.LeafNode.Remotes, remotes) {
		return
	}
	// ReloadOptions diffs against the Server's own stored Options, so the
	// value handed to it must be a genuinely different object - mutating
	// h.opts in place first would make old == new and reload a no-op.
	newOpts := *h.opts
	newOpts.LeafNode.Remotes = remotes
	if err := h.srv.ReloadOptions(&newOpts); err != nil {
		log.Printf("nats: reload leaf remotes: %v", err)
		return
	}
	h.opts = &newOpts
}

func sameRemotes(a, b []*server.RemoteLeafOpts) bool {
	if len(a) != len(b) {
		return false
	}
	want := map[string]bool{}
	for _, r := range a {
		want[r.URLs[0].String()] = true
	}
	for _, r := range b {
		if !want[r.URLs[0].String()] {
			return false
		}
	}
	return true
}

func (h *NatsHub) Shutdown() {
	if h == nil {
		return
	}
	h.nc.Close()
	h.srv.Shutdown()
}
