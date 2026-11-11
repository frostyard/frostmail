package activity_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// TestAnEntryHasNoStringField is ADR-0055's first rule: every field of
// api.ActivityEntry is a number, a time or an enum, so no text can travel
// in one. An enum is a named string type of package api with a Valid
// method.
func TestAnEntryHasNoStringField(t *testing.T) {
	typ := reflect.TypeFor[api.ActivityEntry]()
	valid := reflect.TypeFor[interface{ Valid() bool }]()
	for f := range typ.Fields() {
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Int64, reflect.Bool:
		case reflect.Struct:
			if ft.PkgPath() != "time" || ft.Name() != "Time" {
				t.Errorf("field %s is a %s, not a number, a time or an enum", f.Name, ft)
			}
		case reflect.String:
			if ft.PkgPath() != typ.PkgPath() || ft.Name() == "string" || !ft.Implements(valid) {
				t.Errorf("field %s is text (%s), not an enum", f.Name, ft)
			}
		default:
			t.Errorf("field %s is a %s, not a number, a time or an enum", f.Name, ft)
		}
	}
}

// TestOnlyThisPackageMakesEntries is ADR-0055's second rule: outside
// internal/activity, no production code builds an api.ActivityEntry, with a
// composite literal or new, so every entry comes from a constructor here.
func TestOnlyThisPackageMakesEntries(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "third_party", "node_modules", "build", ".git", "app", "api":
				return filepath.SkipDir
			}
			if path == filepath.Join(root, "internal", "activity") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if isEntry(n.Type) {
					t.Errorf("%s builds an api.ActivityEntry; use internal/activity's constructors", fset.Position(n.Pos()))
				}
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "new" && len(n.Args) == 1 && isEntry(n.Args[0]) {
					t.Errorf("%s makes an api.ActivityEntry with new", fset.Position(n.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Fatalf("checked only %d files; is the module root %s?", checked, root)
	}
}

func isEntry(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "api" && sel.Sel.Name == "ActivityEntry"
}
