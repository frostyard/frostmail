//go:build linux && integration

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoLiteralLooks holds rule 2 of plan 0016 for this client: its code
// takes a kit part, an NSGo control or a token, and writes no look as a
// number. It reads the package's own files, not the app.
//
// A color, a text size and a corner are not written at all. A padding
// still is in the places paddingLeft counts: the kit has no spacing to
// take in their place yet (plan 0016, Phase 4e). A file's count there may
// only go down, and the rule is whole when the list is empty.
func TestNoLiteralLooks(t *testing.T) {
	never := []struct {
		what string
		re   *regexp.Regexp
	}{
		{"a color written as a number", regexp.MustCompile(`core\.RGBA?\(|\b0x[0-9a-fA-F]{6,8}\b`)},
		{"a text size written as a number", regexp.MustCompile(`FontSize:\s*[0-9]|ui\.Text\(.*,\s*[0-9]+,\s*ui\.`)},
		{"a corner written as a number", regexp.MustCompile(`CornerRadius:\s*[0-9]`)},
	}
	padding := regexp.MustCompile(`Padding:\s*core\.(All|Symmetric|Edges)`)
	paddingLeft := map[string]int{
		"activity.go": 1, "calendar_grid.go": 2, "compose.go": 1, "list.go": 2, "modules.go": 1,
		"pim.go": 1, "reader_pane.go": 1, "shell.go": 3, "todo.go": 1,
	}
	dir := filepath.Join(repoRoot(t), "cmd", "frostmail-nsgo")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files in %s: %v", dir, err)
	}
	seen := map[string]int{}
	for _, path := range files {
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, rule := range never {
				if rule.re.MatchString(line) {
					t.Errorf("%s:%d: %s: take a token of the kit or of NSGo", name, n+1, rule.what)
				}
			}
			seen[name] += len(padding.FindAllString(line, -1))
		}
	}
	for name, got := range seen {
		if want := paddingLeft[name]; got > want {
			t.Errorf("%s writes %d paddings as numbers, and %d are allowed: take a kit part or token", name, got, want)
		} else if got < want {
			t.Errorf("%s writes %d paddings as numbers, fewer than the %d listed: lower its count in paddingLeft", name, got, want)
		}
	}
	for name := range paddingLeft {
		if _, ok := seen[name]; !ok {
			t.Errorf("paddingLeft lists %s, which is not a file of the client", name)
		}
	}
}
