package main

// Deriving a type model from Go source (not reflection: reflection sees a
// live value's runtime shape, not the declared one, and can't read doc
// comments). The same AST-walking approach as internal/doclint, applied to
// struct types instead of comments.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// module is this repo's Go module path; every TypeRef.Package must be it or
// a subpackage of it, so we can resolve source directories without
// consulting the build system.
const module = "d8z/gw"

// TypeRef names the types to generate from one package.
type TypeRef struct {
	Package string
	Names   []string
}

// typeKind is a field's or element's shape, independent of Go's own type
// syntax.
type typeKind int

const (
	kindString typeKind = iota // Go string
	kindInt                    // any Go int/uint size
	kindFloat                  // float32 or float64
	kindBool                   // Go bool
	kindTime                   // time.Time
	kindAny                    // any / interface{}
	kindSlice                  // []T
	kindMap                    // map[string]T
	kindRef                    // a named struct type, generated alongside this one
)

// fieldType is a field's (or a slice/map's element's) type.
type fieldType struct {
	kind typeKind
	elem *fieldType // for kindSlice (item) and kindMap (value; key is always string)
	ref  string     // for kindRef: the referenced type's name
}

// field is one exported, JSON-visible struct field, or - embed non-"" - an
// anonymous field whose own fields are promoted (flattened) into this
// struct's, the same way encoding/json treats Go's struct embedding; the
// other field values are unused in that case.
type field struct {
	goName   string
	jsonName string
	doc      string
	typ      fieldType
	optional bool // omitempty, or the field itself is a pointer
	embed    string
}

// structType is one generated type: its fields, in declaration order, and
// its own doc comment.
type structType struct {
	name   string
	doc    string
	fields []field
}

// pkgDir is import path pkg's directory relative to the cwd schemagen runs
// from (the module root, server/) - our own module, so no build-system
// lookup is needed.
func pkgDir(pkg string) (string, error) {
	if pkg == module {
		return ".", nil
	}
	rel, ok := strings.CutPrefix(pkg, module+"/")
	if !ok {
		return "", fmt.Errorf("import path %q is not under module %q", pkg, module)
	}
	return rel, nil
}

// loadTypes resolves every TypeRef to its structType, following references
// to other named struct types in the same package so they're generated
// too (e.g. a field of type []HubImage pulls in HubImage). Returns them in
// a stable, dependency-respecting order (each type after what it refs).
func loadTypes(refs []TypeRef) ([]structType, error) {
	var order []string
	seen := map[string]structType{}
	for _, r := range refs {
		dir, err := pkgDir(r.Package)
		if err != nil {
			return nil, err
		}
		decls, err := parseStructs(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Package, err)
		}
		var resolve func(name string) error
		resolve = func(name string) error {
			if _, ok := seen[name]; ok {
				return nil
			}
			st, ok := decls[name]
			if !ok {
				return fmt.Errorf("%s: no struct type %q", r.Package, name)
			}
			flat, err := flattenEmbeds(st, decls, map[string]bool{name: true})
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			st.fields = flat
			seen[name] = st // mark before recursing, so a cycle doesn't loop
			for _, f := range st.fields {
				if ref := refName(f.typ); ref != "" {
					if err := resolve(ref); err != nil {
						return err
					}
				}
			}
			order = append(order, name)
			return nil
		}
		for _, name := range r.Names {
			if err := resolve(name); err != nil {
				return nil, err
			}
		}
	}
	out := make([]structType, len(order))
	for i, name := range order {
		out[i] = seen[name]
	}
	return out, nil
}

// refName is t's referenced type name, following slice/map elements - "" if
// t (or its element) isn't a kindRef.
func refName(t fieldType) string {
	switch t.kind {
	case kindRef:
		return t.ref
	case kindSlice, kindMap:
		return refName(*t.elem)
	default:
		return ""
	}
}

// flattenEmbeds replaces every embedding marker in st.fields with the
// embedded type's own (recursively flattened) fields, in place - the same
// promotion encoding/json does when marshaling a struct with an anonymous
// field. visiting guards against an embedding cycle.
func flattenEmbeds(st structType, decls map[string]structType, visiting map[string]bool) ([]field, error) {
	var out []field
	for _, f := range st.fields {
		if f.embed == "" {
			out = append(out, f)
			continue
		}
		if visiting[f.embed] {
			return nil, fmt.Errorf("embedding cycle involving %q", f.embed)
		}
		emb, ok := decls[f.embed]
		if !ok {
			return nil, fmt.Errorf("embedded type %q not found in this package", f.embed)
		}
		visiting[f.embed] = true
		embFields, err := flattenEmbeds(emb, decls, visiting)
		if err != nil {
			return nil, err
		}
		delete(visiting, f.embed)
		out = append(out, embFields...)
	}
	return out, nil
}

// parseStructs parses every non-test *.go file in dir and returns its
// exported-to-JSON struct types, by name.
func parseStructs(dir string) (map[string]structType, error) {
	fset := token.NewFileSet()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]structType{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				doc := ts.Doc
				if doc == nil {
					doc = gd.Doc
				}
				fields, err := parseFields(st)
				if err != nil {
					return nil, fmt.Errorf("%s: %s: %w", name, ts.Name.Name, err)
				}
				out[ts.Name.Name] = structType{name: ts.Name.Name, doc: commentText(doc), fields: fields}
			}
		}
	}
	return out, nil
}

// parseFields reads st's exported, JSON-visible fields in declaration
// order. An embedded (anonymous) field becomes an embed marker, flattened
// later by flattenEmbeds; an unexported field or one tagged json:"-" is
// skipped - encoding/json never marshals the former regardless of tag.
func parseFields(st *ast.StructType) ([]field, error) {
	var out []field
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			name, err := embeddedTypeName(f.Type)
			if err != nil {
				return nil, err
			}
			out = append(out, field{embed: name})
			continue
		}
		goName := f.Names[0].Name
		if !ast.IsExported(goName) {
			continue
		}
		jsonName, omitempty, skip := jsonTagName(f.Tag, goName)
		if skip {
			continue
		}
		ft, optional, err := convertType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", goName, err)
		}
		doc := f.Doc
		if doc == nil {
			doc = f.Comment
		}
		out = append(out, field{
			goName: goName, jsonName: jsonName, doc: commentText(doc),
			typ: ft, optional: optional || omitempty,
		})
	}
	return out, nil
}

// embeddedTypeName is an anonymous field's own type name - the name
// flattenEmbeds looks it up by.
func embeddedTypeName(e ast.Expr) (string, error) {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name, nil
	case *ast.StarExpr:
		return embeddedTypeName(t.X)
	default:
		return "", fmt.Errorf("unsupported embedded field type %T", e)
	}
}

// jsonTagName is field goName's JSON name and omitempty-ness from its
// struct tag (Go's own default when there's no json tag: the field name,
// always included). skip is true for json:"-".
func jsonTagName(tag *ast.BasicLit, goName string) (name string, omitempty, skip bool) {
	if tag == nil {
		return goName, false, false
	}
	raw, err := strconv.Unquote(tag.Value)
	if err != nil {
		return goName, false, false
	}
	jsonTag := reflect.StructTag(raw).Get("json")
	if jsonTag == "-" {
		return "", false, true
	}
	parts := strings.Split(jsonTag, ",")
	name = parts[0]
	if name == "" {
		name = goName
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

// convertType maps a Go field type expression to a fieldType. optional is
// true when e is a pointer (nullable, and not required even without
// omitempty).
func convertType(e ast.Expr) (fieldType, bool, error) {
	switch t := e.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return fieldType{kind: kindString}, false, nil
		case "bool":
			return fieldType{kind: kindBool}, false, nil
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64":
			return fieldType{kind: kindInt}, false, nil
		case "float32", "float64":
			return fieldType{kind: kindFloat}, false, nil
		case "any":
			return fieldType{kind: kindAny}, false, nil
		default:
			return fieldType{kind: kindRef, ref: t.Name}, false, nil
		}
	case *ast.InterfaceType:
		return fieldType{kind: kindAny}, false, nil
	case *ast.ArrayType:
		elem, _, err := convertType(t.Elt)
		if err != nil {
			return fieldType{}, false, err
		}
		return fieldType{kind: kindSlice, elem: &elem}, false, nil
	case *ast.MapType:
		elem, _, err := convertType(t.Value)
		if err != nil {
			return fieldType{}, false, err
		}
		return fieldType{kind: kindMap, elem: &elem}, false, nil
	case *ast.StarExpr:
		elem, _, err := convertType(t.X)
		if err != nil {
			return fieldType{}, false, err
		}
		return elem, true, nil
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "time" && t.Sel.Name == "Time" {
			return fieldType{kind: kindTime}, false, nil
		}
		return fieldType{kind: kindAny}, false, nil
	default:
		return fieldType{kind: kindAny}, false, nil
	}
}

// commentText is cg's text with its trailing newline trimmed ("" for nil).
func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}
