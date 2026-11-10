// Command schemagen derives JSON Schema and TypeScript from the Go structs
// that define gw-server's JSON wire format, so they stay the single
// source of truth instead of console/src/types.ts hand-mirroring them.
// Run via `go generate ./...` from server/ (see the module root's
// go:generate directive), or standalone:
//
//	go run ./cmd/schemagen -out-schema ./schema -out-ts ../console/src/types.gen.ts
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// exportedTypes is every type schemagen generates, by the package that
// declares it. Add a type here to generate it - the same idea as the main
// package's own hand-maintained job catalog.
var exportedTypes = []TypeRef{
	{Package: "d8z/gw/deployment", Names: []string{
		"DeploymentResponse", "HubRepoInfo", "MessageResult", "HubInput", "GCPInput", "HubImage",
	}},
	{Package: "d8z/gw/jobs", Names: []string{"JobsResponse", "RunsResponse"}},
}

// main parses flags and generates exportedTypes into them.
func main() {
	outSchema := flag.String("out-schema", "schema", "directory to write <Type>.schema.json files into")
	outTS := flag.String("out-ts", "../console/src/types.gen.ts", "file to write the generated TypeScript into")
	flag.Parse()

	if err := run(*outSchema, *outTS); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

// run resolves exportedTypes and writes both outputs.
func run(outSchema, outTS string) error {
	types, err := loadTypes(exportedTypes)
	if err != nil {
		return err
	}
	if err := writeSchemas(outSchema, types); err != nil {
		return err
	}
	return os.WriteFile(outTS, []byte(renderTS(types)), 0o644)
}

// writeSchemas writes one <Type>.schema.json per type into dir.
func writeSchemas(dir string, types []structType) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, st := range types {
		b, err := marshalSchema(schemaFor(st))
		if err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, st.name+".schema.json"), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
