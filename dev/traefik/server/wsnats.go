package main

// Workspace control plane over NATS, riding the leaf links nats.go already
// builds along the gateway cascade (a parent dials each running child).
//
//   ctl.<child>.assign          child -> parent, request/reply: "what's my
//                               workspace?" The parent answers with the
//                               workspace's repo, branch, mount and GitHub
//                               token, sealed with the child's key.
//   ctl.<child>.assign.changed  parent -> child: the workspace was edited
//                               (new token, branch, ...) - ask again.
//   ctl.<child>.status          parent -> child, request/reply: uncommitted
//                               and unpushed work in the workspace - asked
//                               before the console stops the gateway.
//
// <child> is the child's GW_ID in subject form (natsSubjectID). These live
// outside "gw.<id>.>" on purpose: no JetStream stream captures them, so no
// request or reply ever lands on disk or in a console's message feed.
//
// Why sealed: every leaf link shares one subject space, so any gateway on
// the cascade - or a dind Deightz, which can reach its own traefik's leaf
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

	"github.com/nats-io/nats.go"
)

func ctlSubject(id, verb string) string { return "ctl." + natsSubjectID(id) + "." + verb }

// --- keys and sealing --------------------------------------------------------

var masterKeyOnce struct {
	sync.Once
	key []byte
}

// masterKey is this manager's secret, kept beside workspace.yaml (same
// volume, same "outside every repository" rule) so children's keys survive a
// restart of this gateway.
func (m *Manager) masterKey() []byte {
	masterKeyOnce.Do(func() {
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

func seal(keyHex string, plain []byte) (string, error) {
	g, err := gcmFor(keyHex)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, plain, nil)), nil
}

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

type assignReply struct {
	None   bool   `json:"none,omitempty"`   // that gateway serves no workspace
	Sealed string `json:"sealed,omitempty"` // the wsAssignment, sealed with its key
	Error  string `json:"error,omitempty"`
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

func (m *Manager) answerAssignment(ctx context.Context, name string) assignReply {
	if !nameRE.MatchString(name) {
		return assignReply{Error: "bad gateway name"}
	}
	a, err := m.childAssignment(ctx, name)
	if err != nil {
		return assignReply{Error: err.Error()}
	}
	if a == nil {
		return assignReply{None: true}
	}
	b, _ := json.Marshal(a)
	s, err := seal(m.childKey(m.childID(name)), b)
	if err != nil {
		return assignReply{Error: err.Error()}
	}
	return assignReply{Sealed: s}
}

// childAssignment builds what child `name` gets told, from its labels (which
// workspace, which docker mode) and the current workspace.yaml - so it's
// always current, and survives restarts of either side.
func (m *Manager) childAssignment(ctx context.Context, name string) (*wsAssignment, error) {
	cs, err := childGateways(ctx, m.cfg.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		l := c.Config.Labels
		if l[labelName] != name {
			continue
		}
		if l[labelWorkspace] == "" {
			return nil, nil
		}
		w, err := m.ws.get(l[labelWorkspace])
		if err != nil {
			return nil, err
		}
		mt := m.hostMount(w)
		if l[labelDocker] == "dind" {
			// Inside its sidecar the workspace is a plain directory.
			mt = wsMount{Type: "bind", Source: mt.Target, Target: mt.Target}
		}
		return &wsAssignment{Workspace: w, Token: wsToken(w), Dir: mt.Target, Mount: mt}, nil
	}
	return nil, fmt.Errorf("no child gateway %q", name)
}

// workspaceGateway is the running child serving workspace wsName, if any.
func (m *Manager) workspaceGateway(ctx context.Context, wsName string) string {
	cs, _ := childGateways(ctx, m.cfg.ID)
	for _, c := range cs {
		if c.Config.Labels[labelWorkspace] == wsName {
			return c.Config.Labels[labelName]
		}
	}
	return ""
}

// notifyAssignmentChanged tells a workspace's running gateway to re-fetch.
func (m *Manager) notifyAssignmentChanged(wsName string) {
	if m.nats == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if name := m.workspaceGateway(ctx, wsName); name != "" {
		m.nats.nc.Publish(ctlSubject(m.childID(name), "assign.changed"), nil)
		m.nats.nc.FlushTimeout(2 * time.Second)
	}
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

// runWorkspaceGateway is a workspace gateway's bootstrap: fetch the
// assignment (retrying until our parent has linked to us), then keep it
// current and answer status requests.
func (m *Manager) runWorkspaceGateway() {
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
			log.Printf("workspace %s: %v (retrying)", m.cfg.WorkspaceName, err)
			lastErr = err.Error()
		}
		select {
		case <-changed:
		case <-time.After(3 * time.Second):
		}
	}
}

func (m *Manager) fetchAssignment(first bool) error {
	msg, err := m.nats.nc.Request(ctlSubject(m.cfg.ID, "assign"), nil, 3*time.Second)
	if err != nil {
		return fmt.Errorf("waiting for the parent over NATS: %v", err)
	}
	var rep assignReply
	if err := json.Unmarshal(msg.Data, &rep); err != nil {
		return err
	}
	switch {
	case rep.Error != "":
		return errors.New("parent: " + rep.Error)
	case rep.None:
		return errors.New("parent says this gateway serves no workspace")
	}
	plain, err := unseal(m.cfg.WorkspaceKey, rep.Sealed)
	if err != nil {
		return fmt.Errorf("can't unseal the assignment: %v", err)
	}
	var a wsAssignment
	if err := json.Unmarshal(plain, &a); err != nil {
		return err
	}
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
	Error     string   `json:"error,omitempty"`
}

func (m *Manager) workspaceStatus(ctx context.Context) wsStatus {
	st := wsStatus{Workspace: m.cfg.WorkspaceName, Persists: true}
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
	g := gitCmd{ctx: ctx, log: nil, env: gitEnv(""), pre: []string{"-C", a.Dir}, editor: true}
	// Not g.out: it trims, and a porcelain line's first column can be a space.
	cmd := g.command([]string{"status", "--porcelain=v1", "--untracked-files=normal"})
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
	if n, err := g.out("rev-list", "--count", "HEAD", "--not", "--remotes"); err == nil {
		st.Unpushed, _ = strconv.Atoi(n)
	}
	st.Clean = st.Changed == 0 && st.Untracked == 0 && st.Unpushed == 0
	return st
}

// startWorkspaceNats wires this gateway's part of the control plane.
func (m *Manager) startWorkspaceNats() {
	if m.nats == nil {
		if m.cfg.WorkspaceName != "" {
			log.Printf("workspace %s: no NATS, so no assignment can arrive", m.cfg.WorkspaceName)
		}
		return
	}
	if m.cfg.WorkspaceName != "" {
		go m.runWorkspaceGateway()
		return
	}
	m.serveAssignments()
}
