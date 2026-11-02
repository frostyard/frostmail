//go:build linux

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// The tests of the release version stamp (packaging/flatpak/stamp-version.sh,
// plan 0013, Phase 8) are two: a run of the script on copies of the committed
// manifest and metainfo, and a dry run of the flatpak job's step that calls it,
// parsed out of .github/workflows/flatpak.yml. So they check what a v* tag
// writes into the Flatpak, without a registry and without a Flatpak build. A
// candidate vX.Y.Z-rc.N is the build that Promote releases as vX.Y.Z, so the
// step stamps X.Y.Z, never the -rc.N part.

const (
	stampManifest = "org.frostyard.Frostmail.json"
	stampMetainfo = "org.frostyard.Frostmail.metainfo.xml"
	// stampName is the flatpak job's step that stamps the version.
	stampName = "Stamp the release version"
	// stampCommit is the commit every case stamps, as github's GITHUB_SHA.
	stampCommit = "0123456789abcdef0123456789abcdef01234567"
)

// stampInputs copies the Flatpak's manifest and metainfo into a temporary
// directory, so that stamp-version.sh edits the copies and not the tree.
func stampInputs(t *testing.T) (dir, script string) {
	t.Helper()
	src := filepath.Join("..", "packaging", "flatpak")
	script = filepath.Join(src, "stamp-version.sh")
	dir = t.TempDir()
	for _, name := range []string{stampManifest, stampMetainfo} {
		writeFile(t, dir, name, readFile(t, src, name))
	}
	return dir, script
}

// stampVersion runs stamp-version.sh on dir.
func stampVersion(t *testing.T, dir, script, version, commit string) (string, error) {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "sh", script, version, commit, dir).CombinedOutput()
	return string(out), err
}

// readString reads path, or fails the test.
func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// indent is the leading whitespace of line.
func indent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// utcDays are the UTC dates a stamp run may write: the one before it and the
// one after it. They differ only when the run crosses UTC midnight.
func utcDays(before, after time.Time) []string {
	days := []string{before.Format("2006-01-02")}
	if d := after.Format("2006-01-02"); d != days[0] {
		days = append(days, d)
	}
	return days
}

// releaseLine is the metainfo's release line for version, in each of days.
func releaseLine(version string, days []string) []string {
	lines := make([]string, 0, len(days))
	for _, d := range days {
		lines = append(lines, `<release version="`+version+`" date="`+d+`"/>`)
	}
	return lines
}

// releaseLines is the metainfo's lines and the index of its <releases>.
func releaseLines(t *testing.T, meta string) ([]string, int) {
	t.Helper()
	lines := strings.Split(meta, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "<releases>" {
			return lines, i
		}
	}
	t.Fatalf("no <releases> in the metainfo:\n%s", meta)
	return nil, -1
}

// A v* tag stamps its version into the manifest and the metainfo, so that the
// Flatpak's maild reports the release and software centres show it.
func TestFlatpakStampWritesTheTagIntoTheFlatpak(t *testing.T) {
	dir, script := stampInputs(t)
	before := time.Now().UTC()
	out, err := stampVersion(t, dir, script, "1.2.3", "0123456789ab")
	days := utcDays(before, time.Now().UTC())
	if err != nil {
		t.Fatalf("stamp 1.2.3: %v\n%s", err, out)
	}

	manifest := readString(t, filepath.Join(dir, stampManifest))
	if want := "-X main.version=1.2.3 -X main.commit=0123456789ab"; !strings.Contains(manifest, want) {
		t.Errorf("the manifest does not hold %q:\n%s", want, manifest)
	}
	if strings.Contains(manifest, "-X main.version=flatpak") {
		t.Error("the manifest still names flatpak")
	}

	meta := readString(t, filepath.Join(dir, stampMetainfo))
	lines, at := releaseLines(t, meta)
	if at+2 >= len(lines) {
		t.Fatalf("no <releases> with a child in the metainfo:\n%s", meta)
	}
	want := releaseLine("1.2.3", days)
	if got := strings.TrimSpace(lines[at+1]); !slices.Contains(want, got) {
		t.Errorf("the first release = %q, want %q", got, strings.Join(want, " or "))
	}
	if got, want := indent(lines[at+1]), indent(lines[at+2]); got != want {
		t.Errorf("the new release is indented %q, the one it precedes %q", got, want)
	}
	if !strings.Contains(meta, `<release version="0.0.0"`) {
		t.Error("the metainfo lost its 0.0.0 release")
	}
}

// The script refuses input that no tag produces, and leaves the files as
// they were.
func TestFlatpakStampRefusesBadArguments(t *testing.T) {
	for _, tc := range []struct{ name, version, commit string }{
		{"two parts", "1.2", "0123456789ab"},
		{"four parts", "1.2.3.4", "0123456789ab"},
		{"a v", "v1.2.3", "0123456789ab"},
		{"uppercase commit", "1.2.3", "0123456789AB"},
		{"short commit", "1.2.3", "012345"},
		{"long commit", "1.2.3", strings.Repeat("a", 41)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, script := stampInputs(t)
			out, err := stampVersion(t, dir, script, tc.version, tc.commit)
			if err == nil {
				t.Fatalf("stamp %q %q succeeded:\n%s", tc.version, tc.commit, out)
			}
			if !strings.Contains(out, "stamp-version.sh:") {
				t.Errorf("no message for %q %q:\n%s", tc.version, tc.commit, out)
			}
			manifest := readString(t, filepath.Join(dir, stampManifest))
			if !strings.Contains(manifest, "-X main.version=flatpak") {
				t.Error("the manifest changed under bad arguments")
			}
		})
	}
}

// A second run with a version the metainfo already lists fails, so that a
// re-run cannot list one release twice.
func TestFlatpakStampRefusesToStampTwice(t *testing.T) {
	dir, script := stampInputs(t)
	if out, err := stampVersion(t, dir, script, "1.2.3", "0123456789ab"); err != nil {
		t.Fatalf("first stamp: %v\n%s", err, out)
	}
	out, err := stampVersion(t, dir, script, "1.2.3", "0123456789ab")
	if err == nil {
		t.Fatalf("a second stamp succeeded:\n%s", out)
	}
	meta := readString(t, filepath.Join(dir, stampMetainfo))
	if n := strings.Count(meta, `<release version="1.2.3"`); n != 1 {
		t.Errorf("the metainfo lists 1.2.3 %d times, want once", n)
	}
}

// The metainfo's own record of the version is what stops a second run, even
// when the manifest is fresh again.
func TestFlatpakStampRefusesAVersionAlreadyListed(t *testing.T) {
	dir, script := stampInputs(t)
	meta := readString(t, filepath.Join(dir, stampMetainfo))
	meta = strings.Replace(meta, `    <release version="0.0.0"`,
		"    <release version=\"1.2.3\" date=\"2026-01-01\"/>\n    <release version=\"0.0.0\"", 1)
	writeFile(t, dir, stampMetainfo, meta)
	out, err := stampVersion(t, dir, script, "1.2.3", "0123456789ab")
	if err == nil {
		t.Fatalf("stamp listed 1.2.3 twice:\n%s", out)
	}
	if !strings.Contains(out, "already lists version 1.2.3") {
		t.Errorf("the message does not name the listed version:\n%s", out)
	}
}

// stampSteps is the flatpak job's steps of .github/workflows/flatpak.yml, in
// order.
func stampSteps(t *testing.T) []publishStep {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "flatpak.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []publishStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("flatpak.yml: %v", err)
	}
	job, ok := wf.Jobs["flatpak"]
	if !ok || len(job.Steps) == 0 {
		t.Fatal("flatpak.yml has no flatpak job with steps")
	}
	return job.Steps
}

// stampStep is the one step that stamps the release version.
func stampStep(t *testing.T) publishStep {
	t.Helper()
	var found []publishStep
	for _, s := range stampSteps(t) {
		if s.Name == stampName {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the flatpak job has %d steps named %q, want exactly 1", len(found), stampName)
	}
	return found[0]
}

// A v* tag stamps the version before the build reads the manifest, so that the
// built Flatpak carries the release, and no other run stamps at all.
func TestFlatpakStampStepComesBeforeTheBuild(t *testing.T) {
	steps := stampSteps(t)
	step := stampStep(t)
	if want := "github.event_name == 'push' && startsWith(github.ref, 'refs/tags/v')"; step.If != want {
		t.Errorf("the stamp step's if = %q, want %q", step.If, want)
	}
	build := slices.IndexFunc(steps, func(s publishStep) bool {
		return strings.Contains(s.Uses, "flatpak/flatpak-github-actions/flatpak-builder")
	})
	if build < 0 {
		t.Fatal("the flatpak job never builds the Flatpak")
	}
	stamped := slices.IndexFunc(steps, func(s publishStep) bool { return s.Name == stampName })
	if stamped > build {
		t.Fatalf("the stamp step is at %d, the build at %d: the stamp must come first", stamped, build)
	}
}

// stampSubst replaces the ${{ ... }} expressions of the step's env: the tag and
// the commit. Any other expression fails the test.
func stampSubst(t *testing.T, v, tag string) string {
	t.Helper()
	var unknown string
	out := expr.ReplaceAllStringFunc(v, func(e string) string {
		switch name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(e, "${{"), "}}")); name {
		case "github.ref_name":
			return tag
		case "github.sha":
			return stampCommit
		default:
			unknown = name
			return e
		}
	})
	if unknown != "" {
		t.Fatalf("the expression %q is new to the dry run: teach the dry run this expression", unknown)
	}
	return out
}

// stampWork stages a work dir with the commit's script, manifest and metainfo,
// as a checkout of the repository has them.
func stampWork(t *testing.T) (work string) {
	t.Helper()
	src := filepath.Join("..", "packaging", "flatpak")
	work = t.TempDir()
	for _, name := range []string{"stamp-version.sh", stampManifest, stampMetainfo} {
		writeFile(t, work, "packaging/flatpak/"+name, readFile(t, src, name))
	}
	script := filepath.Join(work, "packaging", "flatpak", "stamp-version.sh")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	return work
}

// stampRun runs the stamp step in work, as GitHub runs it for tag.
func stampRun(t *testing.T, step publishStep, work, tag string) (string, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash", "-e", "-c", step.Run)
	cmd.Dir = work
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range step.Env {
		cmd.Env = append(cmd.Env, k+"="+stampSubst(t, v, tag))
	}
	return runScript(t, cmd)
}

// A candidate tag stamps its release version, not the -rc.N part: Promote
// releases that build as vX.Y.Z, so v0.4.10-rc.12 is 0.4.10.
func TestFlatpakStampStepStampsTheReleaseVersion(t *testing.T) {
	step := stampStep(t)
	for _, tc := range []struct{ tag, version string }{
		{"v0.4.1-rc.2", "0.4.1"},
		{"v0.4.1", "0.4.1"},
		{"v0.4.10-rc.12", "0.4.10"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			work := stampWork(t)
			before := time.Now().UTC()
			out, code := stampRun(t, step, work, tc.tag)
			days := utcDays(before, time.Now().UTC())
			if code != 0 {
				t.Fatalf("tag %s failed with %d:\n%s", tc.tag, code, out)
			}

			meta := readFile(t, work, "packaging/flatpak/"+stampMetainfo)
			lines, at := releaseLines(t, meta)
			if at+1 >= len(lines) {
				t.Fatalf("no release line in the metainfo:\n%s", meta)
			}
			want := releaseLine(tc.version, days)
			if got := strings.TrimSpace(lines[at+1]); !slices.Contains(want, got) {
				t.Errorf("tag %s wrote the first release %q, want %q", tc.tag, got, strings.Join(want, " or "))
			}

			manifest := readFile(t, work, "packaging/flatpak/"+stampManifest)
			if want := "-X main.version=" + tc.version + " -X main.commit=" + stampCommit[:12]; !strings.Contains(manifest, want) {
				t.Errorf("tag %s leaves no %q in the manifest:\n%s", tc.tag, want, manifest)
			}
			if strings.Contains(manifest, "-X main.version=flatpak") {
				t.Errorf("tag %s leaves the manifest naming flatpak", tc.tag)
			}
		})
	}
}

// A tag that is neither vX.Y.Z nor vX.Y.Z-rc.N stops the job before the build,
// and leaves the committed manifest and metainfo as they are.
func TestFlatpakStampStepRefusesOtherTags(t *testing.T) {
	step := stampStep(t)
	src := filepath.Join("..", "packaging", "flatpak")
	for _, tag := range []string{"v0.4", "v0.4.1-beta.1", "v0.4.1-rc", "v0.4.1-rc.1-x", "vnext", "v0.4.1.1"} {
		t.Run(tag, func(t *testing.T) {
			work := stampWork(t)
			out, code := stampRun(t, step, work, tag)
			if code == 0 {
				t.Fatalf("tag %s was stamped:\n%s", tag, out)
			}
			for _, name := range []string{stampManifest, stampMetainfo} {
				if got, want := readFile(t, work, "packaging/flatpak/"+name), readFile(t, src, name); got != want {
					t.Errorf("tag %s changed %s", tag, name)
				}
			}
		})
	}
}
