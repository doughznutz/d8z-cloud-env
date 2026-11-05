package main

// Requests up the tree. A child never acts on its parent: it can only ask,
// and you decide on the parent's console.
//
//   ctl.<child>.request   child -> parent, request/reply: {"kind","reason"}.
//                         The parent queues it (one per child, newest wins)
//                         and acks; nothing happens until it's approved.
//
// Only "restart" exists so far: stop the child's bundle and start it again
// from the same spec, picking up whatever its images are now.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type ParentRequest struct {
	Gateway string    `json:"gateway"` // the child's name (our gw/<name>)
	Kind    string    `json:"kind"`
	Reason  string    `json:"reason,omitempty"`
	Time    time.Time `json:"time"`
}

var requestKinds = map[string]string{"restart": "restart"}

// lookupChildSpec is childSpec, swappable in tests (it asks docker).
var lookupChildSpec = (*Manager).childSpec

// serveRequests queues our direct children's requests.
func (m *Manager) serveRequests() {
	pattern := "ctl.*.request"
	if m.cfg.ID != "root" {
		pattern = "ctl." + natsSubjectID(m.cfg.ID) + ".*.request"
	}
	_, err := m.nats.nc.Subscribe(pattern, func(msg *nats.Msg) {
		toks := strings.Split(msg.Subject, ".")
		name := toks[len(toks)-2]
		rep := map[string]string{"state": "queued"}
		if err := m.queueRequest(name, msg.Data); err != nil {
			rep = map[string]string{"error": err.Error()}
		}
		b, _ := json.Marshal(rep)
		msg.Respond(b)
	})
	if err != nil {
		log.Printf("nats: serve requests: %v", err)
	}
}

func (m *Manager) queueRequest(name string, data []byte) error {
	if !nameRE.MatchString(name) {
		return errors.New("bad gateway name")
	}
	var in struct {
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	if _, ok := requestKinds[in.Kind]; !ok {
		return fmt.Errorf("unknown request %q", in.Kind)
	}
	if len(in.Reason) > 500 {
		in.Reason = in.Reason[:500]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := lookupChildSpec(m, ctx, name); err != nil {
		return err
	}
	m.mu.Lock()
	if m.requests == nil {
		m.requests = map[string]*ParentRequest{}
	}
	m.requests[name] = &ParentRequest{Gateway: name, Kind: in.Kind, Reason: in.Reason, Time: time.Now().UTC()}
	m.mu.Unlock()
	note := fmt.Sprintf("gw/%s asks for a %s", name, in.Kind)
	if in.Reason != "" {
		note += ": " + in.Reason
	}
	m.nats.Hello(note + " (approve it on the Control tab)")
	return nil
}

// Requests lists the queued requests, oldest first.
func (m *Manager) Requests() []ParentRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ParentRequest, 0, len(m.requests))
	for _, r := range m.requests {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

func (m *Manager) takeRequest(name string) (*ParentRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.requests[name]
	if r == nil {
		return nil, fmt.Errorf("no request from gw/%s", name)
	}
	delete(m.requests, name)
	return r, nil
}

// childSpec rebuilds the spawn request a running child was started with,
// from its labels.
func (m *Manager) childSpec(ctx context.Context, name string) (SpawnRequest, error) {
	cs, err := childGateways(ctx, m.cfg.ID)
	if err != nil {
		return SpawnRequest{}, err
	}
	for _, c := range cs {
		l := c.Config.Labels
		if l[labelName] != name {
			continue
		}
		req := SpawnRequest{Name: name, Docker: l[labelDocker], Project: l[labelSrcProject], Workspace: l[labelWorkspace], Branch: l[labelBranch]}
		if l[labelProfiles] != "" {
			req.Profiles = strings.Split(l[labelProfiles], ",")
		}
		return req, nil
	}
	return SpawnRequest{}, fmt.Errorf("no child gateway %q", name)
}

// Restart stops a child and starts it again from the same spec - and with
// its config changes, saved as a branch first and started from.
func (m *Manager) Restart(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	req, err := m.childSpec(ctx, name)
	cancel()
	if err != nil {
		return err
	}
	cfgLog := newRingBuf(16 << 10)
	if id := m.takeChildConfig(cfgLog, name, "branch"); id != "" {
		req.Branch = id
	}
	if err := m.Stop(name, "discard"); err != nil { // already saved
		return err
	}
	go func() {
		// Stop drops its op once the bundle is gone (and keeps it on error).
		for i := 0; i < 600; i++ {
			time.Sleep(time.Second)
			m.mu.Lock()
			o := m.ops[name]
			state := ""
			if o != nil {
				state = o.State
			}
			m.mu.Unlock()
			if state == "error" {
				log.Printf("restart %s: stop failed, not starting it again", name)
				return
			}
			if o == nil {
				if err := m.Spawn(context.Background(), req); err != nil {
					log.Printf("restart %s: %v", name, err)
					m.mu.Lock()
					m.ops[name] = &op{State: "error", Err: "restart: " + err.Error(), Req: req, Log: newRingBuf(4 << 10)}
					m.mu.Unlock()
				}
				return
			}
		}
	}()
	return nil
}

// AskParent sends a request up to our parent.
func (m *Manager) AskParent(kind, reason string) error {
	if m.cfg.Role == "root" {
		return errors.New("the root has no parent to ask")
	}
	if m.nats == nil {
		return errors.New("nats not available")
	}
	b, _ := json.Marshal(map[string]string{"kind": kind, "reason": reason})
	msg, err := m.nats.nc.Request(ctlSubject(m.cfg.ID, "request"), b, 5*time.Second)
	if err != nil {
		return fmt.Errorf("parent didn't answer over NATS: %v", err)
	}
	var rep map[string]string
	json.Unmarshal(msg.Data, &rep)
	if rep["error"] != "" {
		return errors.New("parent: " + rep["error"])
	}
	m.nats.Hello(fmt.Sprintf("asked the parent for a %s", kind))
	return nil
}

func registerRequestRoutes(mux *http.ServeMux, m *Manager) {
	// Ours to the parent.
	mux.HandleFunc("POST /api/parent/requests", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := m.AskParent(in.Kind, in.Reason); err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "queued at the parent"})
	})
	// Our children's, to us.
	mux.HandleFunc("GET /api/requests", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"requests": m.Requests()})
	})
	mux.HandleFunc("POST /api/requests/{name}/approve", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		req, err := m.takeRequest(name)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if err := m.Restart(name); err != nil {
			m.mu.Lock()
			m.requests[name] = req // keep it: nothing happened
			m.mu.Unlock()
			writeErr(w, http.StatusConflict, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "restarting"})
	})
	mux.HandleFunc("DELETE /api/requests/{name}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := m.takeRequest(r.PathValue("name")); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"state": "dismissed"})
	})
}
