// Command doclint checks every gw-server package's doc comments for
// problems golangci-lint's godoclint can't see (internal/doclint): an
// orphaned file-overview comment, an HTTP route missing from its
// registering function's doc, an uncommented lock, or a comment naming a
// .go file that no longer exists. It's a build step of its own, like
// golangci-lint or go vet, rather than something riding on go test.
//
// Run from server/:
//
//	go run ./cmd/doclint         # fails (exit 1) on any new problem
//	go run ./cmd/doclint -update # rewrites every package's baseline instead
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"d8z/gw/internal/doclint"
)

// packages is every package doclint checks, as a directory relative to
// server/ - the same hand-maintained-list idea as schemagen's own
// exportedTypes.
var packages = []string{
	".",
	"cfgfile",
	"cfgtree",
	"optracker",
	"deployment",
	"jobs",
	"internal/apiclient",
	"internal/doclint",
	"cmd/schemagen",
	"cmd/doclint",
}

// main checks (or, with -update, rewrites) every package in packages.
func main() {
	update := flag.Bool("update", false, "rewrite every package's testdata/doclint_baseline.txt instead of checking it")
	flag.Parse()

	failed := false
	for _, dir := range packages {
		if err := checkOne(dir, *update); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

// checkOne verifies (or, with update, rewrites) dir's baseline, prefixing
// every message it returns with dir so a mixed-package failure list stays
// readable.
func checkOne(dir string, update bool) error {
	baseline := filepath.Join(dir, "testdata", "doclint_baseline.txt")
	msgs, err := doclint.Verify(dir, baseline, update)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if len(msgs) == 0 {
		return nil
	}
	out := dir + ":"
	for _, m := range msgs {
		out += "\n  " + m
	}
	return fmt.Errorf("%s", out)
}
