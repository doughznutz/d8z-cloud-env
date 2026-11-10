package main

import (
	"fmt"
	"strings"
)

// safeProjectName rules out path traversal in a single directory name under
// the projects root (a local workspace, a bundle source). Deliberately
// lenient otherwise: real checkouts have underscores, dots, mixed case.
func safeProjectName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid directory name %q", name)
	}
	return nil
}
