// Package cfgfile writes gateway config files (workspace.yaml,
// deployment.yaml, a key) atomically, and notifies the gateway tree when one
// changes. It has no dependency on Manager or any particular file's shape -
// every caller hands it bytes already rendered.
package cfgfile

import (
	"os"
	"path/filepath"
	"syscall"
)

// OnWrite runs after every config file write: our children rebase onto the
// new config (Manager.configChanged).
var OnWrite func()

// WriteFile writes a config file atomically with mode 0600, keeping an
// existing file's owner - or else its directory's - so a file on the host
// stays editable by its user.
func WriteFile(file string, content []byte) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	uid, gid := -1, -1
	if st, err := os.Stat(file); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(sys.Uid), int(sys.Gid)
		}
	} else if st, err := os.Stat(dir); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(sys.Uid), int(sys.Gid)
		}
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	os.Chown(tmp, uid, gid)
	if err := os.Rename(tmp, file); err != nil {
		// The file itself is bind-mounted (not its directory): can't
		// replace it, so rewrite it in place.
		os.Remove(tmp)
		if err := os.WriteFile(file, content, 0o600); err != nil {
			return err
		}
	}
	if OnWrite != nil {
		OnWrite()
	}
	return nil
}
