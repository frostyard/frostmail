package workflowcheck

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestOnlyImapxImportsGoIMAP keeps the production import boundary of AGENTS.md.
func TestOnlyImapxImportsGoIMAP(t *testing.T) {
	root := filepath.Join("..", "..")
	parsed := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && (skipped(d.Name(), nil) || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, prefix := range []string{"internal/imapx/", "third_party/", "tools/msprobe/"} {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		parsed++
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if path == "github.com/emersion/go-imap" || strings.HasPrefix(path, "github.com/emersion/go-imap/") {
				t.Errorf("%s imports %s: only internal/imapx may import go-imap", rel, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed == 0 {
		t.Fatal("parsed 0 Go files: check the repository root")
	}
}
