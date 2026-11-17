package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// P-957: every window of the app draws at the user's text size, so the
// content of each mygo.NewWindow goes through windowSet.sized. A window
// added without it keeps the usual size, whatever the user chose.
func TestEveryWindowDrawsThroughSized(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "NewWindow" {
				return true
			}
			found++
			where := fset.Position(call.Pos()).String()
			sized := false
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, el := range lit.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok || !isIdent(kv.Key, "Content") {
						continue
					}
					ast.Inspect(kv.Value, func(m ast.Node) bool {
						if c, ok := m.(*ast.CallExpr); ok {
							if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "sized" {
								sized = true
							}
						}
						return !sized
					})
				}
			}
			if !sized {
				t.Errorf("%s: the window's Content does not go through windowSet.sized", where)
			}
			return true
		})
	}
	if found != 4 {
		t.Errorf("found %d windows, want the main, compose, Calendar and settings windows", found)
	}
}

// sized sets the text size from the scale it is given, before the view.
func TestSizedAdaptsBeforeTheView(t *testing.T) {
	scale := float32(1.3)
	s := &windowSet{textScale: func() float32 { return scale }}
	var size float32
	tt := ui.NewTester(s.sized(func(c *ui.Context) {
		size = c.Theme().FontSize
		ui.Text(c, "x")
	}), 100, 50)
	tt.Frame()
	usual := ui.LightTheme().FontSize
	if d := size - usual*1.3; d > 0.001 || d < -0.001 {
		t.Errorf("the view saw FontSize %v, want %v", size, usual*1.3)
	}
	scale = 1
	tt.Frame()
	if size != usual {
		t.Errorf("at the usual size the view saw %v, want %v", size, usual)
	}
	// Until the main window exists there is no scale: the usual size.
	s.textScale = nil
	tt.Frame()
	if size != usual {
		t.Errorf("with no scale the view saw %v, want %v", size, usual)
	}
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}
