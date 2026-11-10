// Package optracker tracks background operations that aren't gateway
// spawns: local service start/stop ("svc:<name>") and jobs ("job:<id>"),
// so the console can show progress and logs before - or without - any
// container to show it. It has its own mutex, independent of whatever else
// a caller (Manager) also tracks.
package optracker

import (
	"fmt"
	"strings"
	"sync"
)

// Op tracks an in-flight (or finished) operation.
type Op struct {
	State string   // starting | stopping | error | done
	Phase string   // human-readable step while starting/stopping
	Err   string   // the failure, when State is "error"
	Log   *RingBuf // the op's output, for the console
}

// Tracker is a set of Ops by key ("svc:<name>", "job:<id>"), guarded by its
// own mutex.
type Tracker struct {
	mu  sync.Mutex // guards ops and every Op's fields
	ops map[string]*Op
}

// New makes an empty Tracker.
func New() *Tracker { return &Tracker{ops: map[string]*Op{}} }

// Log is an op's log by key.
func (t *Tracker) Log(name string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if o := t.ops[name]; o != nil {
		return o.Log.String(), true
	}
	return "", false
}

// Start starts tracking a new in-flight operation, refusing if one is
// already running under this key.
func (t *Tracker) Start(name, state, phase string) (*Op, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if o := t.ops[name]; o != nil && (o.State == "starting" || o.State == "stopping") {
		return nil, fmt.Errorf("%s is busy (%s)", strings.TrimPrefix(strings.TrimPrefix(name, "svc:"), "job:"), o.State)
	}
	o := &Op{State: state, Phase: phase, Log: newRingBuf(64 << 10)}
	t.ops[name] = o
	return o, nil
}

// Finish records the outcome of the op started under name. On error the op
// is kept with its error; a successful job (name has the "job:" prefix) is
// kept as "done" so its output stays readable; any other successful op is
// dropped.
func (t *Tracker) Finish(name string, o *Op, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		o.State, o.Err = "error", err.Error()
		fmt.Fprintf(o.Log, "\nERROR: %v\n", err)
		return
	}
	fmt.Fprintln(o.Log, "\ndone.")
	if strings.HasPrefix(name, "job:") {
		// A job's output is its result - keep it readable after success.
		o.State, o.Phase = "done", ""
		return
	}
	delete(t.ops, name)
}

// Snapshot reports state/phase/error for every tracked op whose key has
// prefix, keyed by the rest of the key (the prefix stripped) - e.g.
// Snapshot("job:") for every job's current run.
func (t *Tracker) Snapshot(prefix string) map[string]map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := map[string]map[string]string{}
	for key, o := range t.ops {
		id, ok := strings.CutPrefix(key, prefix)
		if !ok {
			continue
		}
		v := map[string]string{"state": o.State}
		if o.Phase != "" {
			v["phase"] = o.Phase
		}
		if o.Err != "" {
			v["error"] = o.Err
		}
		out[id] = v
	}
	return out
}

// RingBuf keeps the tail of an op's output.
type RingBuf struct {
	mu  sync.Mutex // guards buf
	max int
	buf []byte
}

// newRingBuf makes a RingBuf keeping the last max bytes.
func newRingBuf(max int) *RingBuf { return &RingBuf{max: max} }

// Write appends p, dropping the oldest bytes beyond max. It never fails.
func (r *RingBuf) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if over := len(r.buf) - r.max; over > 0 {
		r.buf = r.buf[over:]
	}
	return len(p), nil
}

// String is the buffered tail ("" for a nil RingBuf).
func (r *RingBuf) String() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
