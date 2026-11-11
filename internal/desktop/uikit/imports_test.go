package uikit_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allowed are uikit's permitted imports beyond the standard library, so a
// pattern can move into frostui by changing an import path (plan 0015).
var allowed = []string{"github.com/egoist/mygo", "github.com/ebitengine/purego", "github.com/frostyard/frostui"}

func TestUikitImportsOnlyWhatFrostuiCan(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			first, _, _ := strings.Cut(path, "/")
			if !strings.Contains(first, ".") {
				continue // the standard library
			}
			ok := false
			for _, a := range allowed {
				ok = ok || path == a || strings.HasPrefix(path, a+"/")
			}
			if !ok {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}
