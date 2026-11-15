package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// source is one Go package's parsed source, for its doc comments and
// constants.
type source struct {
	fset  *token.FileSet
	types map[string]*doc.Type
	// consts are the package's constant declarations, wherever go/doc
	// filed them.
	consts []*doc.Value
}

// source parses the package with import path pkg, from this module.
func (g *generator) source(pkg string) (*source, error) {
	if s, ok := g.sources[pkg]; ok {
		return s, nil
	}
	dir, err := packageDir(pkg)
	if err != nil {
		return nil, err
	}
	s, err := parseSource(dir, pkg)
	if err != nil {
		return nil, err
	}
	g.sources[pkg] = s
	return s, nil
}

// packageDir is the folder of package pkg in the module the working
// folder is in.
func packageDir(pkg string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for line := range strings.Lines(string(mod)) {
				if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					path = strings.Trim(strings.TrimSpace(path), `"`)
					if rest, ok := strings.CutPrefix(pkg, path); ok && (rest == "" || rest[0] == '/') {
						return filepath.Join(dir, filepath.FromSlash(rest)), nil
					}
					return "", fmt.Errorf("package %s isn't in module %s", pkg, path)
				}
			}
			return "", fmt.Errorf("%s has no module line", filepath.Join(dir, "go.mod"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("the working folder isn't in a Go module")
		}
		dir = parent
	}
}

// parseSource parses the Go files in dir that build for this platform,
// test files included so that a test's fixture types have their docs,
// and reads them with go/doc.
func parseSource(dir, pkg string) (*source, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	name := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, e.Name()); err != nil || !ok {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(f.Name.Name, "_test") {
			continue // an external test package
		}
		name = f.Name.Name
		files[path] = f
	}
	// Not doc.NewFromFiles: it leaves test files' declarations out.
	p := doc.New(&ast.Package{Name: name, Files: files}, pkg, doc.AllDecls|doc.PreserveAST)
	s := &source{fset: fset, types: map[string]*doc.Type{}, consts: p.Consts}
	for _, t := range p.Types {
		s.types[t.Name] = t
		s.consts = append(s.consts, t.Consts...)
	}
	return s, nil
}

// typeDoc is the doc comment of the type called name.
func (s *source) typeDoc(name string) string {
	if t, ok := s.types[name]; ok {
		return t.Doc
	}
	return ""
}

// fieldDocs are the doc comments of the fields of the struct called
// name, by Go field name; a field with no doc comment has its line
// comment.
func (s *source) fieldDocs(name string) map[string]string {
	out := map[string]string{}
	t, ok := s.types[name]
	if !ok {
		return out
	}
	for _, spec := range t.Decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || ts.Name.Name != name {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, f := range st.Fields.List {
			text := f.Doc.Text()
			if text == "" {
				text = f.Comment.Text()
			}
			for _, n := range f.Names {
				out[n.Name] = text
			}
		}
	}
	return out
}

// constant is a declared constant of a typed string.
type constant struct {
	name, value string
}

// constants are the declared constants of the typed string called name.
// A constant declared in a test file counts only when the type is
// declared in one too: a test's constant isn't a value the Core sends.
func (s *source) constants(name string) ([]constant, error) {
	t, ok := s.types[name]
	if !ok {
		return nil, fmt.Errorf("no source declares %s", name)
	}
	var out []constant
	var errs []error
	for _, v := range s.consts {
		if s.isTest(v.Decl) && !s.isTest(t.Decl) {
			continue
		}
		consts, err := constantsIn(v.Decl, name)
		out = append(out, consts...)
		errs = append(errs, err)
	}
	return out, errors.Join(errs...)
}

// constantsIn are the constants of type name that decl declares, typed
// as `X T = "x"`, converted as `X = T("x")`, or repeating the line above.
func constantsIn(decl *ast.GenDecl, name string) ([]constant, error) {
	var out []constant
	var errs []error
	var prevType string
	var prevValues []ast.Expr
	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		typ, values := "", vs.Values
		switch {
		case vs.Type != nil:
			if id, ok := vs.Type.(*ast.Ident); ok {
				typ = id.Name
			}
		case len(vs.Values) == 0:
			typ, values = prevType, prevValues
		}
		prevType, prevValues = typ, values
		for i, n := range vs.Names[:min(len(vs.Names), len(values))] {
			value, conv := values[i], ""
			if call, ok := value.(*ast.CallExpr); ok && len(call.Args) == 1 {
				if id, ok := call.Fun.(*ast.Ident); ok {
					value, conv = call.Args[0], id.Name
				}
			}
			if typ != name && conv != name {
				continue
			}
			lit, ok := value.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				errs = append(errs, fmt.Errorf("can't read the value of %s's constant %s: it isn't a string literal", name, n.Name))
				continue
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			out = append(out, constant{n.Name, text})
		}
	}
	return out, errors.Join(errs...)
}

// isTest reports whether decl is in a test file.
func (s *source) isTest(decl *ast.GenDecl) bool {
	return strings.HasSuffix(s.fset.File(decl.Pos()).Name(), "_test.go")
}
