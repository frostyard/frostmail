package workflowcheck

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// skipped reports whether the repository walk leaves directory name out:
// version control, dependencies, build output, and each name in extra.
func skipped(name string, extra map[string]bool) bool {
	switch name {
	case ".git", "node_modules", "build", "dist", "target":
		return true
	}
	return extra[name]
}

// repoFiles returns every file of the repository as a path from its root with
// forward slashes. The directories skipped, and each of extra, are left out.
func repoFiles(t *testing.T, extra ...string) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	skip := make(map[string]bool, len(extra))
	for _, name := range extra {
		skip[name] = true
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipped(d.Name(), skip) {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestInstructionAliases guards row B1 of frostyard/core
// docs/specs/repository-baseline.md: AGENTS.md is the one instruction file,
// and every other agent entry point is a relative symlink to it.
func TestInstructionAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows checkout has no symlinks")
	}
	agents := filepath.Join("..", "..", "AGENTS.md")
	fi, err := os.Lstat(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("AGENTS.md is not a regular file (mode %s); it is the one instruction file", fi.Mode())
	}
	want, err := filepath.EvalSymlinks(agents)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"CLAUDE.md",
		"GEMINI.md",
		".cursorrules",
		filepath.Join(".github", "copilot-instructions.md"),
	} {
		if got, ok := relativeSymlink(t, name); ok && got != want {
			t.Errorf("%s resolves to %q, want %q", name, got, want)
		}
	}

	name := filepath.Join(".claude", "skills")
	got, ok := relativeSymlink(t, name)
	wantSkills, err := filepath.EvalSymlinks(filepath.Join("..", "..", ".agents", "skills"))
	if err != nil {
		t.Fatalf(".agents/skills: %v", err)
	}
	if ok && got != wantSkills {
		t.Errorf("%s resolves to %q, want %q", name, got, wantSkills)
	}
	if fi, err := os.Stat(filepath.Join("..", "..", name)); err != nil {
		t.Errorf("%s: %v", name, err)
	} else if !fi.IsDir() {
		t.Errorf("%s resolves to a file, want the directory .agents/skills", name)
	}
}

// relativeSymlink reports whether name is a symlink whose target is relative,
// and returns the path it resolves to.
func relativeSymlink(t *testing.T, name string) (string, bool) {
	t.Helper()
	link := filepath.Join("..", "..", name)
	fi, err := os.Lstat(link)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return "", false
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink", name)
		return "", false
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return "", false
	}
	if filepath.IsAbs(target) {
		t.Errorf("%s links to the absolute path %q; it must be relative", name, target)
		return "", false
	}
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return "", false
	}
	return got, true
}

// correctionsMember names the 5 members a line of .memory/corrections.jsonl
// carries (row B2).
var correctionsMember = []string{"date", "scope", "correction", "evidence", "promoted_to"}

// TestCorrectionsInbox guards row B2 of frostyard/core
// docs/specs/repository-baseline.md: .memory/corrections.jsonl holds one JSON
// object per line, with exactly the 5 fields of a correction.
func TestCorrectionsInbox(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".memory", "corrections.jsonl"))
	if err != nil {
		t.Fatalf(".memory/corrections.jsonl: %v", err)
	}
	if len(data) == 0 {
		t.Fatal(".memory/corrections.jsonl is empty")
	}
	lines := strings.Split(string(data), "\n")
	if n := len(lines); lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if len(lines) == 0 {
		t.Fatal(".memory/corrections.jsonl holds no line")
	}
	date := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for i, line := range lines {
		n := i + 1
		if strings.TrimSpace(line) == "" {
			t.Errorf("line %d: blank line", n)
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("line %d: %v", n, err)
			continue
		}
		for _, name := range correctionsMember {
			v, ok := m[name]
			if !ok {
				t.Errorf("line %d: no %q member", n, name)
				continue
			}
			s, ok := v.(string)
			if !ok {
				t.Errorf("line %d: %q is not a string", n, name)
				continue
			}
			if s == "" {
				t.Errorf("line %d: %q is empty", n, name)
			}
		}
		for name := range m {
			if !slices.Contains(correctionsMember, name) {
				t.Errorf("line %d: unknown member %q", n, name)
			}
		}
		if s, ok := m["date"].(string); ok && !date.MatchString(s) {
			t.Errorf("line %d: date %q is not YYYY-MM-DD", n, s)
		}
	}
}

// riskPath is 1 entry of .github/risk-paths.json.
type riskPath struct {
	Glob string `json:"glob"`
	Why  string `json:"why"`
}

// riskPathFile is the whole of .github/risk-paths.json.
type riskPathFile struct {
	SchemaVersion int        `json:"schema_version"`
	Paths         []riskPath `json:"paths"`
}

// governedPaths are the release, secrets and governance paths of this
// repository; .github/risk-paths.json must list each of them.
var governedPaths = []string{
	".github/risk-paths.json",
	".github/workflows/risk-paths.yml",
	"Makefile",
	"mise.toml",
	"mise.lock",
	".svu.yml",
	"packaging/flatpak/org.frostyard.Frostmail.json",
	"packaging/nfpm.yaml",
	"packaging/lodge/make-secrets-key.sh",
	"packaging/lodge/frostmail.service",
	"scripts/seal-mail-secrets.sh",
	"scripts/move-mail-to-lodge.sh",
	"internal/secrets/sealed.go",
	"api/scope.go",
}

// TestRiskPaths guards row B6 of frostyard/core
// docs/specs/repository-baseline.md: .github/risk-paths.json lists every
// governed path of the repository, with a reason, and no stale glob.
func TestRiskPaths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "risk-paths.json"))
	if err != nil {
		t.Fatalf(".github/risk-paths.json: %v", err)
	}
	var cfg riskPathFile
	if err := json.Unmarshal(data, &cfg, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf(".github/risk-paths.json: %v", err)
	}
	if cfg.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", cfg.SchemaVersion)
	}
	if len(cfg.Paths) == 0 {
		t.Fatal(".github/risk-paths.json lists no path")
	}
	globs := make([]string, 0, len(cfg.Paths))
	seen := make(map[string]bool, len(cfg.Paths))
	for i, p := range cfg.Paths {
		if err := validGlob(p.Glob); err != nil {
			t.Errorf("entry %d: %v", i, err)
		}
		if seen[p.Glob] {
			t.Errorf("entry %d: glob %q appears twice", i, p.Glob)
		}
		seen[p.Glob] = true
		globs = append(globs, p.Glob)
		if strings.TrimSpace(p.Why) == "" {
			t.Errorf("entry %d: glob %q has no why", i, p.Glob)
		}
		if n := utf8.RuneCountInString(p.Why); n > 200 {
			t.Errorf("entry %d: glob %q has a why of %d characters, want 200 at most", i, p.Glob, n)
		}
	}

	want := append([]string{}, governedPaths...)
	workflows, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range workflows {
		want = append(want, path.Join(".github/workflows", filepath.Base(p)))
	}
	for _, p := range want {
		if !matchAny(globs, p) {
			t.Errorf("%s is a release, secrets or governance path; .github/risk-paths.json must list it", p)
		}
	}

	files := repoFiles(t)
	for _, g := range globs {
		if !anyMatch(g, files) {
			t.Errorf("glob %q matches no file in the repository", g)
		}
	}
}

// validGlob reports why glob is not a path glob of frostyard/core
// docs/specs/risk-paths.md, or nil when it is one.
func validGlob(glob string) error {
	if glob == "" {
		return errors.New("empty glob")
	}
	if strings.HasPrefix(glob, "/") || strings.HasPrefix(glob, "./") {
		return fmt.Errorf("glob %q must be relative to the repository root", glob)
	}
	for _, seg := range strings.Split(glob, "/") {
		if seg == "" || seg == ".." {
			return fmt.Errorf("glob %q has an empty or .. segment", glob)
		}
	}
	return nil
}

// matchAny reports whether p matches 1 of globs.
func matchAny(globs []string, p string) bool {
	for _, g := range globs {
		if matchGlob(g, p) {
			return true
		}
	}
	return false
}

// anyMatch reports whether glob matches 1 of paths.
func anyMatch(glob string, paths []string) bool {
	for _, p := range paths {
		if matchGlob(glob, p) {
			return true
		}
	}
	return false
}

// matchGlob reports whether glob matches the whole slash-separated path
// (frostyard/core docs/specs/risk-paths.md): {a,b} is 1 of the alternatives,
// ** is 0 or more whole segments, * and ? never cross a slash, and every
// other character is literal.
func matchGlob(glob, p string) bool {
	for _, alt := range expandBraces(glob) {
		if globRegexp(alt).MatchString(p) {
			return true
		}
	}
	return false
}

// expandBraces returns glob with each {a,b,c} group replaced by its
// alternatives in turn. Groups do not nest.
func expandBraces(glob string) []string {
	open := strings.IndexByte(glob, '{')
	if open < 0 {
		return []string{glob}
	}
	end := strings.IndexByte(glob[open:], '}')
	if end < 0 {
		return []string{glob}
	}
	end += open
	var out []string
	for _, alt := range strings.Split(glob[open+1:end], ",") {
		out = append(out, expandBraces(glob[:open]+alt+glob[end+1:])...)
	}
	return out
}

// globRegexp compiles 1 brace-free glob, which holds no { and no }, into the
// regular expression that matches the whole path.
func globRegexp(glob string) *regexp.Regexp {
	if glob == "**" {
		return regexp.MustCompile(`^.*$`)
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); {
		switch {
		case strings.HasPrefix(glob[i:], "/**/"):
			b.WriteString(`/(?:[^/]+/)*`)
			i += len("/**/")
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString(`(?:[^/]+/)*`)
			i += len("**/")
		case strings.HasPrefix(glob[i:], "/**") && i+len("/**") == len(glob):
			b.WriteString(`(?:/.*)?`)
			i += len("/**")
		case glob[i] == '*':
			b.WriteString(`[^/]*`)
			i++
		case glob[i] == '?':
			b.WriteString(`[^/]`)
			i++
		default:
			j := i
			for j < len(glob) && glob[j] != '*' && glob[j] != '?' {
				j++
			}
			b.WriteString(regexp.QuoteMeta(glob[i:j]))
			i = j
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// TestMatchGlob checks the glob rules of frostyard/core
// docs/specs/risk-paths.md on the cases .github/risk-paths.json uses.
func TestMatchGlob(t *testing.T) {
	for _, tc := range []struct {
		glob string
		path string
		want bool
	}{
		{".github/workflows/**", ".github/workflows/ci.yml", true},
		{".github/workflows/**", ".github/risk-paths.json", false},
		{"{mise.toml,mise.lock}", "mise.toml", true},
		{"{mise.toml,mise.lock}", "mise.lock", true},
		{"{mise.toml,mise.lock}", "mise.tom", false},
		{"scripts/{a,b}.sh", "scripts/a.sh", true},
		{"scripts/{a,b}.sh", "scripts/c.sh", false},
		{"Makefile", "Makefile", true},
		{"Makefile", "app/Makefile", false},
		{"**/x", "x", true},
		{"**/x", "a/b/x", true},
		{"a/*", "a/b", true},
		{"a/*", "a/b/c", false},
		{"**", "a/b", true},
	} {
		if got := matchGlob(tc.glob, tc.path); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.glob, tc.path, got, tc.want)
		}
	}
}

// dependabotUpdate is 1 entry of the updates list of
// .github/dependabot.yml.
type dependabotUpdate struct {
	Ecosystem string `yaml:"package-ecosystem"`
	Directory string `yaml:"directory"`
}

// dependabotFile is the whole of .github/dependabot.yml.
type dependabotFile struct {
	Version int                `yaml:"version"`
	Updates []dependabotUpdate `yaml:"updates"`
}

// ecosystem names the Dependabot package-ecosystem of the manifest file
// base, if base is a manifest.
func ecosystem(base string) (string, bool) {
	switch base {
	case "go.mod":
		return "gomod", true
	case "package.json":
		return "npm", true
	case "Cargo.toml":
		return "cargo", true
	case "Dockerfile", "Containerfile":
		return "docker", true
	}
	return "", false
}

// normalizeDir drops the trailing slash of a Dependabot directory, and keeps
// the root as "/".
func normalizeDir(dir string) string {
	if dir == "/" {
		return dir
	}
	return strings.TrimSuffix(dir, "/")
}

// TestDependabotCoversManifests guards row B13 of frostyard/core
// docs/specs/repository-baseline.md: .github/dependabot.yml has 1 entry for
// each manifest of the repository, and the github-actions entry at "/".
func TestDependabotCoversManifests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatalf(".github/dependabot.yml: %v", err)
	}
	var cfg dependabotFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf(".github/dependabot.yml: %v", err)
	}
	if cfg.Version != 2 {
		t.Errorf("version = %d, want 2", cfg.Version)
	}
	covered := make(map[[2]string]bool, len(cfg.Updates))
	actions := false
	for _, u := range cfg.Updates {
		dir := normalizeDir(u.Directory)
		covered[[2]string{u.Ecosystem, dir}] = true
		if u.Ecosystem == "github-actions" && dir == "/" {
			actions = true
		}
	}
	if !actions {
		t.Error("no github-actions entry with directory /")
	}

	manifests := 0
	for _, f := range repoFiles(t, "third_party", "docs") {
		eco, ok := ecosystem(path.Base(f))
		if !ok {
			continue
		}
		manifests++
		dir := path.Dir(f)
		want := "/"
		if dir != "." {
			want = "/" + dir
		}
		if !covered[[2]string{eco, want}] {
			t.Errorf("%s has no Dependabot entry (package-ecosystem %s, directory %s)", f, eco, want)
		}
	}
	if manifests == 0 {
		t.Fatal("no dependency manifest found")
	}
}
