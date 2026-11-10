// Package doclint is the checking logic behind cmd/doclint: it catches
// doc-comment problems golangci-lint's godoclint can't, because godoclint
// only sees comments attached to top-level declarations. It's a plain
// library (no *testing.T) - cmd/doclint is what runs it, over its own list
// of package directories, the same way golangci-lint or go vet runs as its
// own build step rather than through go test.
package doclint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// goFileRef matches a bare Go file name mentioned in a comment, such as a
// pointer to the test file that covers something.
var goFileRef = regexp.MustCompile(`\b([A-Za-z0-9_]+\.go)\b`)

// syncType matches the declared type of a lock or other concurrency
// primitive, whose comment must say what it guards.
var syncType = regexp.MustCompile(`^(sync\.(Mutex|RWMutex|Map|Once|WaitGroup|Cond)|atomic\.\w+)`)

// Check returns every doc-comment finding in dir (one package's *.go
// files), sorted and deduplicated:
//
//   - orphan-comment: a top-level comment attached to nothing (e.g. a file
//     overview after the package clause) - go doc drops it entirely.
//   - undocumented-route: a mux.HandleFunc pattern ("GET /api/self") that
//     isn't listed in its registering function's doc comment - the routes
//     are closures, so that comment is all go doc shows of them.
//   - uncommented-sync: a lock (or other sync/atomic value), as a variable
//     or struct field, embedded ones included, with no comment saying what
//     it guards.
//   - missing-file-ref: a comment naming a .go file that isn't in this
//     package (e.g. a test that was renamed or removed).
func Check(dir string) ([]string, error) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
		exists[names[i]] = true
	}
	var found []string
	for i, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		name := names[i]
		found = append(found, orphanComments(name, f)...)
		found = append(found, undocumentedRoutes(name, f)...)
		found = append(found, uncommentedSync(name, f)...)
		found = append(found, missingFileRefs(name, f, exists)...)
	}
	sort.Strings(found)
	return dedupe(found), nil
}

// Verify checks dir against baselinePath, a file listing findings that
// predate these checks, one key per line, '#' comments ignored (a missing
// file is an empty baseline): tolerated until fixed, never added to. It
// returns one message per new finding not in the baseline, and one per
// baseline entry that's now fixed (so the baseline only ever shrinks).
//
// With update true, baselinePath is rewritten from the current findings
// instead, and Verify always returns no messages.
func Verify(dir, baselinePath string, update bool) ([]string, error) {
	found, err := Check(dir)
	if err != nil {
		return nil, err
	}
	if update {
		return nil, writeBaseline(baselinePath, found)
	}
	baseline, err := readBaseline(baselinePath)
	if err != nil {
		return nil, err
	}
	var msgs []string
	current := map[string]bool{}
	for _, k := range found {
		current[k] = true
		if !baseline[k] {
			msgs = append(msgs, fmt.Sprintf("new doc problem: %s", k))
		}
	}
	for k := range baseline {
		if !current[k] {
			msgs = append(msgs, fmt.Sprintf("fixed - remove from %s: %s", baselinePath, k))
		}
	}
	sort.Strings(msgs)
	return msgs, nil
}

// readBaseline parses baselinePath's findings (a missing file is an empty
// baseline, not an error).
func readBaseline(baselinePath string) (map[string]bool, error) {
	baseline := map[string]bool{}
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			return baseline, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			baseline[line] = true
		}
	}
	return baseline, nil
}

// writeBaseline rewrites baselinePath from found.
func writeBaseline(baselinePath string, found []string) error {
	body := "# Doc problems that predate these checks (cmd/doclint): tolerated\n" +
		"# until fixed, never added to. Regenerate after fixing some with:\n" +
		"#   go run ./cmd/doclint -update\n"
	for _, k := range found {
		body += k + "\n"
	}
	return os.WriteFile(baselinePath, []byte(body), 0o644)
}

// orphanComments reports top-level comment groups that are neither the
// package comment, nor a declaration's doc, nor inside a declaration.
func orphanComments(name string, f *ast.File) []string {
	attached := map[*ast.CommentGroup]bool{f.Doc: true}
	ast.Inspect(f, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.FuncDecl:
			attached[d.Doc] = true
		case *ast.GenDecl:
			attached[d.Doc] = true
		case *ast.TypeSpec:
			attached[d.Doc], attached[d.Comment] = true, true
		case *ast.ValueSpec:
			attached[d.Doc], attached[d.Comment] = true, true
		case *ast.Field:
			attached[d.Doc], attached[d.Comment] = true, true
		case *ast.ImportSpec:
			attached[d.Doc], attached[d.Comment] = true, true
		}
		return true
	})
	var out []string
	for _, cg := range f.Comments {
		if attached[cg] || insideDecl(f, cg) || isDirective(cg) || isDivider(cg) {
			continue
		}
		first := strings.Fields(cg.Text())
		if len(first) > 6 {
			first = first[:6]
		}
		out = append(out, fmt.Sprintf("orphan-comment %s: %s", name, strings.Join(first, " ")))
	}
	return out
}

// insideDecl reports whether a comment sits within a declaration's source
// range - in a function body or between a declaration's parts, where it
// explains code rather than documenting an API.
func insideDecl(f *ast.File, cg *ast.CommentGroup) bool {
	for _, d := range f.Decls {
		if cg.Pos() > d.Pos() && cg.End() < d.End() {
			return true
		}
	}
	return false
}

// isDirective reports whether a comment group is only //go: or // +build
// lines, which are instructions to the toolchain, not documentation.
func isDirective(cg *ast.CommentGroup) bool {
	for _, c := range cg.List {
		if !strings.HasPrefix(c.Text, "//go:") && !strings.HasPrefix(c.Text, "// +build") {
			return false
		}
	}
	return true
}

// isDivider reports whether a comment is a section separator such as
// "// --- the parent's side ---" - layout for readers of the source, not
// documentation of anything.
func isDivider(cg *ast.CommentGroup) bool {
	return len(cg.List) == 1 && strings.HasPrefix(strings.TrimSpace(cg.Text()), "---")
}

// undocumentedRoutes reports mux.HandleFunc patterns missing from the doc
// comment of the function that registers them.
func undocumentedRoutes(name string, f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		doc := ""
		if fn.Doc != nil {
			doc = fn.Doc.Text()
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			pattern, err := strconv.Unquote(lit.Value)
			if err == nil && !strings.Contains(doc, pattern) {
				out = append(out, fmt.Sprintf("undocumented-route %s %s: %s", name, fn.Name.Name, pattern))
			}
			return true
		})
	}
	return out
}

// uncommentedSync reports locks and other sync/atomic values - package
// variables and struct fields, embedded or named - that have no comment.
func uncommentedSync(name string, f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				return true
			}
			for _, s := range d.Specs {
				vs := s.(*ast.ValueSpec)
				if vs.Type == nil || !syncType.MatchString(typeString(vs.Type)) {
					continue
				}
				if !hasText(d.Doc) && !hasText(vs.Doc) && !hasText(vs.Comment) {
					out = append(out, fmt.Sprintf("uncommented-sync %s: var %s", name, vs.Names[0].Name))
				}
			}
		case *ast.TypeSpec:
			st, ok := d.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				ts := typeString(field.Type)
				if !syncType.MatchString(ts) || hasText(field.Doc) || hasText(field.Comment) {
					continue
				}
				fname := ts // embedded: named by its type
				if len(field.Names) > 0 {
					fname = field.Names[0].Name
				}
				out = append(out, fmt.Sprintf("uncommented-sync %s: %s.%s", name, d.Name.Name, fname))
			}
		}
		return true
	})
	return out
}

// missingFileRefs reports .go file names mentioned in comments that don't
// exist in this package.
func missingFileRefs(name string, f *ast.File, exists map[string]bool) []string {
	var out []string
	for _, cg := range f.Comments {
		for _, m := range goFileRef.FindAllStringSubmatch(cg.Text(), -1) {
			if !exists[m[1]] {
				out = append(out, fmt.Sprintf("missing-file-ref %s: %s", name, m[1]))
			}
		}
	}
	return out
}

// typeString renders a type expression as source, e.g. "sync.Mutex" or
// "atomic.Pointer[wsAssignment]".
func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.IndexExpr:
		return typeString(t.X) + "[" + typeString(t.Index) + "]"
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	}
	return ""
}

// hasText reports whether a comment group exists and says something.
func hasText(cg *ast.CommentGroup) bool {
	return cg != nil && strings.TrimSpace(cg.Text()) != ""
}

// dedupe drops repeated entries from a sorted list.
func dedupe(sorted []string) []string {
	out := sorted[:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
