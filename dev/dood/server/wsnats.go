package main

// Workspace control plane over NATS, riding the leaf links nats.go already
// builds along the gateway cascade (a parent dials each running child).
//
//   ctl.<child>.assign          child -> parent, request/reply: "what am I
//                               given?" The parent answers with its config
//                               (cfgtree.go) and, for a workspace gateway,
//                               the workspace's repo, branch, mount and
//                               GitHub token - sealed with the child's key.
//   ctl.<child>.assign.changed  parent -> child: our config or the workspace
//                               changed - ask again (and rebase).
//   ctl.<child>.status          parent -> child, request/reply: open work -
//                               uncommitted / unpushed git work in its
//                               workspace, and its config changes - asked
//                               before the console stops the gateway.
//   ctl.<child>.config.diff     parent -> child, request/reply: its config
//                               changes themselves (sealed), to merge or
//                               save as a branch when it's stopped.
//
// <child> is the child's GW_ID in subject form (natsSubjectID). These live
// outside "gw.<id>.>" on purpose: no JetStream stream captures them, so no
// request or reply ever lands on disk or in a console's message feed.
//
// Why sealed: every leaf link shares one subject space, so any gateway on
// the cascade - or a dind Deightz, which can reach its own dood's leaf
// listener - could ask for *another* workspace's assignment. The parent
// derives one key per child (HMAC of its master key and the child's ID),
// hands it to that child alone at spawn (GW_WORKSPACE_KEY), and AES-GCM
// seals each answer with it: anyone else asking gets ciphertext.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"d8z/gw/cfgtree"
	"d8z/gw/jobs"

	"github.com/nats-io/nats.go"
)

// ctlSubject is gateway id's control subject for verb, e.g.
// ctlSubject("abc-xyz", "assign") -> "ctl.abc.xyz.assign".
func ctlSubject(id, verb string) string { return "ctl." + natsSubjectID(id) + "." + verb }

// --- keys and sealing --------------------------------------------------------

// masterKeyOnce holds the master key, loaded or created once per process.
var masterKeyOnce struct {
	sync.Once
	key []byte
}

// masterKey is this manager's secret, kept beside workspace.yaml (same
// volume, same "outside every repository" rule) so children's keys survive a
// restart of this gateway.
func (m *Manager) masterKey() []byte {
	masterKeyOnce.Do(func() {
		// A workspace gateway has no workspace.yaml volume to keep a key in;
		// its children's keys derive from its own key instead, which its
		// parent re-issues on every start - so they survive restarts too.
		if m.cfg.WorkspaceKey != "" {
			mac := hmac.New(sha256.New, []byte(m.cfg.WorkspaceKey))
			mac.Write([]byte("d8z-child-master"))
			masterKeyOnce.key = mac.Sum(nil)
			return
		}
		f := filepath.Join(filepath.Dir(m.cfg.WorkspaceFile), ".gw-master-key")
		if b, err := os.ReadFile(f); err == nil {
			if k, err := hex.DecodeString(strings.TrimSpace(string(b))); err == nil && len(k) == 32 {
				masterKeyOnce.key = k
				return
			}
		}
		k := make([]byte, 32)
		rand.Read(k)
		masterKeyOnce.key = k
		os.MkdirAll(filepath.Dir(f), 0o700)
		if err := os.WriteFile(f, []byte(hex.EncodeToString(k)+"\n"), 0o600); err != nil {
			log.Printf("workspace master key not persisted (%v): running workspace gateways can't re-fetch their assignment after a restart of this one", err)
		}
	})
	return masterKeyOnce.key
}

// childKey is the key a child with GW_ID id unseals its assignment with.
func (m *Manager) childKey(id string) string {
	mac := hmac.New(sha256.New, m.masterKey())
	mac.Write([]byte("d8z-workspace-assign:" + id))
	return hex.EncodeToString(mac.Sum(nil))
}

// gcmFor builds an AES-256-GCM cipher from a hex-encoded 32-byte key.
func gcmFor(keyHex string) (cipher.AEAD, error) {
	k, err := hex.DecodeString(keyHex)
	if err != nil || len(k) != 32 {
		return nil, errors.New("bad workspace key")
	}
	blk, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blk)
}

// seal encrypts plain with keyHex and returns base64(nonce || ciphertext).
func seal(keyHex string, plain []byte) (string, error) {
	g, err := gcmFor(keyHex)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, plain, nil)), nil
}

// unseal reverses seal, failing if the key is wrong or the data was altered.
func unseal(keyHex, sealed string) ([]byte, error) {
	g, err := gcmFor(keyHex)
	if err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(b) < g.NonceSize() {
		return nil, errors.New("malformed sealed assignment")
	}
	return g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
}

// assignReply is the wire reply to an assign or config.diff request: either
// a sealed payload or an error.
type assignReply struct {
	Sealed string `json:"sealed,omitempty"` // the assignGift, sealed with its key
	Error  string `json:"error,omitempty"`
}

// assignGift is everything a child is given by its parent.
type assignGift struct {
	Assign *wsAssignment     `json:"assign,omitempty"` // the workspace it serves, if any
	Files  map[string][]byte `json:"files"`            // the parent's config (cfgtree.go)
	Env    map[string]string `json:"env,omitempty"`    // env vars that config names (token_env)
	Branch []cfgtree.Op      `json:"branch,omitempty"` // config changes it was started from
}

// --- parent side --------------------------------------------------------------

// serveAssignments answers our direct children's assignment requests.
func (m *Manager) serveAssignments() {
	pattern := "ctl.*.assign" // the root's children's IDs are their bare names
	if m.cfg.ID != "root" {
		pattern = "ctl." + natsSubjectID(m.cfg.ID) + ".*.assign"
	}
	_, err := m.nats.nc.Subscribe(pattern, func(msg *nats.Msg) {
		toks := strings.Split(msg.Subject, ".")
		name := toks[len(toks)-2]
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rep := m.answerAssignment(ctx, name)
		b, _ := json.Marshal(rep)
		msg.Respond(b)
	})
	if err != nil {
		log.Printf("nats: serve assignments: %v", err)
	}
}

// answerAssignment builds child name's assignGift (its workspace, our
// config, and the branch it started from) and seals it with that child's key.
func (m *Manager) answerAssignment(ctx context.Context, name string) assignReply {
	if !nameRE.MatchString(name) {
		return assignReply{Error: "bad gateway name"}
	}
	a, branchID, err := m.childAssignment(ctx, name)
	if err != nil {
		return assignReply{Error: err.Error()}
	}
	files, env, err := m.configGift()
	if err != nil {
		return assignReply{Error: err.Error()}
	}
	g := assignGift{Assign: a, Files: files, Env: env}
	if branchID != "" {
		if br, err := m.loadBranch(branchID); err == nil {
			g.Branch = br.Ops
		}
	}
	b, _ := json.Marshal(g)
	s, err := seal(m.childKey(m.childID(name)), b)
	if err != nil {
		return assignReply{Error: err.Error()}
	}
	return assignReply{Sealed: s}
}

// childAssignment builds what child `name` gets told, from its labels (which
// workspace, which docker mode, which branch it started from) and the
// current workspace.yaml - so it's always current, and survives restarts of
// either side.
func (m *Manager) childAssignment(ctx context.Context, name string) (*wsAssignment, string, error) {
	cs, err := childGateways(ctx, m.cfg.ID)
	if err != nil {
		return nil, "", err
	}
	for _, c := range cs {
		l := c.Config.Labels
		if l[labelName] != name {
			continue
		}
		if l[labelWorkspace] == "" {
			return nil, l[labelBranch], nil
		}
		w, mt, token, err := m.workspaceFor(l[labelWorkspace])
		if err != nil {
			return nil, "", err
		}
		if l[labelDocker] == "dind" {
			// Inside its sidecar the workspace is a plain directory.
			mt = wsMount{Type: "bind", Source: mt.Target, Target: mt.Target}
		}
		return &wsAssignment{Workspace: w, Token: token, Dir: mt.Target, Mount: mt}, l[labelBranch], nil
	}
	return nil, "", fmt.Errorf("no child gateway %q", name)
}

// notifyChildren tells every running child to re-fetch what it's given.
func (m *Manager) notifyChildren() {
	if m.nats == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cs, _ := childGateways(ctx, m.cfg.ID)
	for _, c := range cs {
		m.nats.nc.Publish(ctlSubject(m.childID(c.Config.Labels[labelName]), "assign.changed"), nil)
	}
	m.nats.nc.FlushTimeout(2 * time.Second)
}

// ChildConfigDiff asks a child for its config changes.
func (m *Manager) ChildConfigDiff(name string) ([]cfgtree.Op, error) {
	if m.nats == nil {
		return nil, errors.New("nats not available")
	}
	msg, err := m.nats.nc.Request(ctlSubject(m.childID(name), "config.diff"), nil, 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("gw/%s didn't answer over NATS: %v", name, err)
	}
	var rep assignReply
	if err := json.Unmarshal(msg.Data, &rep); err != nil {
		return nil, err
	}
	if rep.Error != "" {
		return nil, errors.New("gw/" + name + ": " + rep.Error)
	}
	plain, err := unseal(m.childKey(m.childID(name)), rep.Sealed)
	if err != nil {
		return nil, err
	}
	var ops []cfgtree.Op
	return ops, json.Unmarshal(plain, &ops)
}

// ChildWorkspaceStatus asks a child for its workspace's open work.
func (m *Manager) ChildWorkspaceStatus(name string) (json.RawMessage, error) {
	if m.nats == nil {
		return nil, errors.New("nats not available")
	}
	if !nameRE.MatchString(name) {
		return nil, errors.New("bad name")
	}
	msg, err := m.nats.nc.Request(ctlSubject(m.childID(name), "status"), nil, 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("gw/%s didn't answer over NATS: %v", name, err)
	}
	return json.RawMessage(msg.Data), nil
}

// --- child side ---------------------------------------------------------------

// runChild is a child gateway's bootstrap: fetch what our parent gives us
// (retrying until it has linked to us), then keep it current and answer its
// status and config-diff requests.
func (m *Manager) runChild() {
	id := m.cfg.ID
	changed := make(chan struct{}, 1)
	poke := func(*nats.Msg) {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	if _, err := m.nats.nc.Subscribe(ctlSubject(id, "assign.changed"), poke); err != nil {
		log.Printf("nats: %v", err)
	}
	if _, err := m.nats.nc.Subscribe(ctlSubject(id, "status"), func(msg *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		b, _ := json.Marshal(m.workspaceStatus(ctx))
		msg.Respond(b)
	}); err != nil {
		log.Printf("nats: %v", err)
	}
	if _, err := m.nats.nc.Subscribe(ctlSubject(id, "config.diff"), func(msg *nats.Msg) {
		rep := assignReply{}
		if ops, err := m.configDiff(); err != nil {
			rep.Error = err.Error()
		} else {
			b, _ := json.Marshal(ops)
			if rep.Sealed, err = seal(m.cfg.WorkspaceKey, b); err != nil {
				rep.Error = err.Error()
			}
		}
		b, _ := json.Marshal(rep)
		msg.Respond(b)
	}); err != nil {
		log.Printf("nats: %v", err)
	}

	first := true
	lastErr := ""
	for {
		err := m.fetchAssignment(first)
		if err == nil {
			first = false
			lastErr = ""
			select {
			case <-changed:
			case <-time.After(30 * time.Minute): // belt and braces: pick up edits even if a notice was missed
			}
			continue
		}
		if err.Error() != lastErr { // don't flood the log while the link comes up
			log.Printf("gw %s: %v (retrying)", id, err)
			lastErr = err.Error()
		}
		select {
		case <-changed:
		case <-time.After(3 * time.Second):
		}
	}
}

// fetchAssignment asks the parent for our assignGift, applies its config,
// and - in a workspace gateway - stores the workspace assignment and
// prepares its directory. first is true for the initial fetch after start:
// that one announces itself in full and pulls an empty workspace; later ones
// pass the refresh on to our own children.
func (m *Manager) fetchAssignment(first bool) error {
	msg, err := m.nats.nc.Request(ctlSubject(m.cfg.ID, "assign"), nil, 3*time.Second)
	if err != nil {
		return fmt.Errorf("waiting for the parent over NATS: %v", err)
	}
	var rep assignReply
	if err := json.Unmarshal(msg.Data, &rep); err != nil {
		return err
	}
	if rep.Error != "" {
		return errors.New("parent: " + rep.Error)
	}
	plain, err := unseal(m.cfg.WorkspaceKey, rep.Sealed)
	if err != nil {
		return fmt.Errorf("can't unseal the assignment: %v", err)
	}
	var g assignGift
	if err := json.Unmarshal(plain, &g); err != nil {
		return err
	}
	if err := m.receiveConfig(g.Files, g.Env, g.Branch); err != nil {
		m.nats.Hello("couldn't take the parent's config: " + err.Error())
		return err
	}
	if m.cfg.WorkspaceName == "" {
		if first {
			m.nats.Hello("config received from the parent")
		}
		return nil
	}
	if g.Assign == nil {
		return errors.New("parent says this gateway serves no workspace")
	}
	a := *g.Assign
	if a.Name != m.cfg.WorkspaceName {
		return fmt.Errorf("parent assigned %q, expected %q", a.Name, m.cfg.WorkspaceName)
	}
	m.assign.Store(&a)
	prepareWorkspaceDir(&a)
	present, _, _ := wsGitState(a.Dir)
	note := fmt.Sprintf("workspace %s assigned: %s %s → %s (token %s)", a.Name, a.Source, firstNonEmpty(a.Repo, a.Path), a.Dir,
		map[bool]string{true: "set", false: "none"}[a.Token != ""])
	if !first {
		note = "workspace " + a.Name + " assignment refreshed"
		go m.notifyChildren() // pass it on to our children
	}
	m.nats.Hello(note)
	log.Print(note)
	// A fresh workspace pulls itself: what to pull and where came with the
	// assignment.
	if first && !present && a.Repo != "" {
		if id, err := m.RunJob("github", "pull", "", nil); err == nil {
			m.nats.Hello("workspace empty: started " + id)
		} else {
			m.nats.Hello("workspace empty, couldn't start pull: " + err.Error())
		}
	}
	return nil
}

// firstNonEmpty returns the first non-empty string in s, or "".
func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// wsStatus is a workspace's open work, as a stop confirmation shows it.
type wsStatus struct {
	Workspace string   `json:"workspace"`
	Assigned  bool     `json:"assigned"`
	Source    string   `json:"source,omitempty"`
	Present   bool     `json:"present"`
	Git       bool     `json:"git"`
	Branch    string   `json:"branch,omitempty"`
	Changed   int      `json:"changed"`   // modified/added/deleted, not committed
	Untracked int      `json:"untracked"` // new files git doesn't know yet
	Unpushed  int      `json:"unpushed"`  // commits on no remote branch
	Files     []string `json:"files,omitempty"`
	Clean     bool     `json:"clean"`
	Persists  bool     `json:"persists"` // stopping keeps the files (volume / host dir) - true today
	Config    []string `json:"config"`   // config changes (cfgtree.go), without values
	Error     string   `json:"error,omitempty"`
}

// workspaceStatus gathers this gateway's open work - config changes and,
// in a workspace gateway, uncommitted, untracked and unpushed git work - to
// answer the parent's status request.
func (m *Manager) workspaceStatus(ctx context.Context) wsStatus {
	st := wsStatus{Workspace: m.cfg.WorkspaceName, Persists: true, Config: []string{}}
	if ops, err := m.configDiff(); err != nil {
		st.Config = []string{"can't diff config: " + err.Error()}
	} else {
		for _, o := range ops {
			st.Config = append(st.Config, o.Summary())
		}
	}
	if m.cfg.WorkspaceName == "" {
		st.Clean = len(st.Config) == 0
		return st
	}
	a := m.assignment()
	if a == nil {
		st.Error = "not assigned yet"
		return st
	}
	st.Assigned, st.Source = true, a.Source
	present, isGit, head := wsGitState(a.Dir)
	st.Present, st.Git, st.Branch = present, isGit, head
	if !isGit {
		st.Clean = !present // nothing to lose; files without git can't be judged
		return st
	}
	g := jobs.GitCmd{Ctx: ctx, Log: nil, Env: jobs.GitEnv(""), Pre: []string{"-C", a.Dir}, Editor: true}
	// Not g.Out: it trims, and a porcelain line's first column can be a space.
	cmd := g.Command([]string{"status", "--porcelain=v1", "--untracked-files=normal"})
	raw, err := cmd.Output()
	out := strings.TrimRight(string(raw), "\n")
	if err != nil {
		st.Error = "git status failed: " + err.Error()
		return st
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		if strings.HasPrefix(line, "??") {
			st.Untracked++
		} else {
			st.Changed++
		}
		if len(st.Files) < 20 {
			st.Files = append(st.Files, line)
		}
	}
	if n, err := g.Out("rev-list", "--count", "HEAD", "--not", "--remotes"); err == nil {
		st.Unpushed, _ = strconv.Atoi(n)
	}
	st.Clean = st.Changed == 0 && st.Untracked == 0 && st.Unpushed == 0 && len(st.Config) == 0
	return st
}

// startWorkspaceNats wires this gateway's part of the control plane.
func (m *Manager) startWorkspaceNats() {
	if m.nats == nil {
		if m.cfg.WorkspaceKey != "" {
			log.Printf("no NATS, so nothing can arrive from the parent")
		}
		return
	}
	// Every gateway answers its own children; every one that has a parent
	// (it was given a key) asks it for its config, and its workspace.
	m.serveAssignments()
	m.serveRequests()
	if m.cfg.WorkspaceKey != "" {
		go m.runChild()
	}
}
