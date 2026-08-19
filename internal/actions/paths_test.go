package actions

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCommandPathsCoverAllConstants(t *testing.T) {
	constants := actionConstants(t)
	if len(constants) == 0 {
		t.Fatal("no action constants found")
	}
	for _, name := range constants {
		if _, ok := commandPaths[name]; !ok {
			t.Errorf("missing command path for action %q", name)
		}
	}
	if len(commandPaths) != len(constants) {
		t.Errorf("commandPaths has %d entries, actions.go has %d constants", len(commandPaths), len(constants))
	}
}

func actionConstants(t *testing.T) []string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src := filepath.Join(filepath.Dir(file), "actions.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				bl, ok := v.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				out = append(out, bl.Value[1:len(bl.Value)-1])
			}
		}
	}
	return out
}
