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
// number: no color, text size, corner or padding. A padding takes the
// kit's spacing (workspaceui.SpaceXS to SpaceXL, and Indent). It reads the
// package's own files, not the app.
func TestNoLiteralLooks(t *testing.T) {
	never := []struct {
		what string
		re   *regexp.Regexp
	}{
		{"a color written as a number", regexp.MustCompile(`core\.RGBA?\(|\b0x[0-9a-fA-F]{6,8}\b`)},
		{"a text size written as a number", regexp.MustCompile(`FontSize:\s*[0-9]|ui\.Text\(.*,\s*[0-9]+,\s*ui\.`)},
		{"a corner written as a number", regexp.MustCompile(`CornerRadius:\s*[0-9]`)},
		// A number anywhere in what a padding is built from, as in
		// core.All(8) or core.Edges{Left: core.Points(depth * 12)}.
		{"a padding written as a number", regexp.MustCompile(`Padding:\s*core\.(All|Symmetric|Edges)[({][^)}]*\b[0-9]`)},
	}
	dir := filepath.Join(repoRoot(t), "cmd", "frostmail-nsgo")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files in %s: %v", dir, err)
	}
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
		}
	}
	// The rule's own patterns find what they are for.
	for _, bad := range []string{"Padding: core.All(8)", "Padding: core.Edges{Left: core.Points(depth * 12)}", "Padding: core.Symmetric(workspaceui.SpaceXS, 6)"} {
		if !never[3].re.MatchString(bad) {
			t.Errorf("the padding rule misses %q", bad)
		}
	}
	for _, good := range []string{"Padding: core.All(workspaceui.SpaceS)", "Padding: core.Edges{Left: workspaceui.Indent(i.Depth)}", "Padding: core.Edges{Left: inset}"} {
		if never[3].re.MatchString(good) {
			t.Errorf("the padding rule refuses %q", good)
		}
	}
}
