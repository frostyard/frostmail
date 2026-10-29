//go:build linux

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The tests of the Flatpak publishing workflow (.github/workflows/flatpak.yml,
// ADR-0045, plan 0013 Phase 8) are a dry run of its publish job: they parse
// the job's steps, run them in order in a temp work dir, and play skopeo and
// gh with stubs that log the command they are given. So they check which OCI
// tags a tag pushes, and that a release candidate leaves latest alone, without
// a registry, a token or a Flatpak build.

const (
	publishImage = "ghcr.io/frostyard/frostmail"
	// The commit every case publishes, as github's GITHUB_SHA.
	publishSHA = "0123456789abcdef0123456789abcdef01234567"
	// The digest the stub skopeo writes to its --digestfile: sha256: and 64 a's.
	publishDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// publishStubs is the stub for both commands: it logs "name args..." as one
// line, and skopeo writes the digest it would have computed.
const publishStubs = `#!/bin/bash
{ printf '%s' "$(basename "$0")"; [ $# -eq 0 ] || printf ' %s' "$@"; printf '\n'; } >>"$PUBLISH_LOG"
if [ "$(basename "$0")" = skopeo ]; then
	prev=
	for arg in "$@"; do
		if [ "$prev" = --digestfile ]; then printf '%s' "$PUBLISH_DIGEST" >"$arg"; fi
		prev=$arg
	done
fi
`

// publishStep is one step of the publish job, as the workflow writes it.
type publishStep struct {
	Name string            `yaml:"name"`
	ID   string            `yaml:"id"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
	Run  string            `yaml:"run"`
}

// label is the step's name, or its action when it has no name.
func (s publishStep) label() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Uses
}

// publishSteps is the publish job's steps, in order.
func publishSteps(t *testing.T) []publishStep {
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
	steps := wf.Jobs["publish"].Steps
	if len(steps) == 0 {
		t.Fatal("flatpak.yml has no publish job with steps")
	}
	return steps
}

// publishRun is one dry run of the publish job, for one tag.
type publishRun struct {
	t       *testing.T
	tag     string
	root    string
	work    string
	log     string
	steps   int
	job     map[string]string
	outputs map[string]map[string]string
	// order is the label of every step the run met, run or skipped.
	order []string
	// attested is the digest the attestation named, or "".
	attested string
}

// newPublishRun stages a work dir and the stub commands for tag.
func newPublishRun(t *testing.T, tag, dispatch string) *publishRun {
	t.Helper()
	root := t.TempDir()
	r := &publishRun{
		t:       t,
		tag:     tag,
		root:    root,
		work:    filepath.Join(root, "work"),
		log:     filepath.Join(root, "log"),
		outputs: map[string]map[string]string{},
		job: map[string]string{
			"IMAGE":      publishImage,
			"GITHUB_SHA": publishSHA,
			// The stubs' log and digest.
			"PUBLISH_LOG":    filepath.Join(root, "log"),
			"PUBLISH_DIGEST": publishDigest,
			"DISPATCH_TOKEN": dispatch,
			"PATH":           filepath.Join(root, "bin") + ":" + os.Getenv("PATH"),
		},
	}
	if err := os.MkdirAll(r.work, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "log", "")
	writeFile(t, root, "bin/stub", publishStubs)
	if err := os.Chmod(filepath.Join(root, "bin", "stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skopeo", "gh"} {
		if err := os.Symlink("stub", filepath.Join(root, "bin", name)); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// run runs the steps in order and stops at the first that fails, as GitHub
// does. It returns the label and the output of the step that failed, or "",
// when every step passed.
func (r *publishRun) run(steps []publishStep) (string, string) {
	r.t.Helper()
	for _, s := range steps {
		r.order = append(r.order, s.label())
		// The condition comes first, so a new one on an action step fails the
		// test too.
		if !r.condition(s.If) {
			continue
		}
		if s.Uses != "" {
			r.attest(s)
			continue
		}
		output := filepath.Join(r.work, "github-output-"+r.slot(s))
		if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
			r.t.Fatal(err)
		}
		cmd := exec.CommandContext(r.t.Context(), "bash", "-e", "-c", s.Run)
		cmd.Dir = r.work
		cmd.Env = r.env(s, output)
		out, code := runScript(r.t, cmd)
		r.readOutput(s.ID, output)
		if code != 0 {
			return s.label(), out
		}
	}
	return "", ""
}

// attest records the attestation the attest step makes: the digest it names,
// which the push step wrote. The other actions are skipped here: the download
// is played by the steps that read the image.
func (r *publishRun) attest(s publishStep) {
	r.t.Helper()
	if !strings.Contains(s.Uses, "attest-build-provenance") {
		return
	}
	if got := r.subst(s.With["subject-name"]); got != publishImage {
		r.t.Fatalf("the attestation names the image %q, want %q", got, publishImage)
	}
	digest := r.subst(s.With["subject-digest"])
	if digest != publishDigest {
		r.t.Fatalf("the attestation names the digest %q, want %q", digest, publishDigest)
	}
	r.attested = digest
}

// slot is a file name piece for a step's GITHUB_OUTPUT, since a step may
// have no id.
func (r *publishRun) slot(s publishStep) string {
	if s.ID != "" {
		return s.ID
	}
	r.steps++
	return "step" + strconv.Itoa(r.steps)
}

// condition evaluates a step's if. It knows the conditions of this workflow
// and fails the test on any other, so a new one is taught to the dry run.
func (r *publishRun) condition(cond string) bool {
	r.t.Helper()
	switch strings.TrimSpace(cond) {
	case "":
		return true
	case "steps.tags.outputs.latest == 'true'":
		return r.outputs["tags"]["latest"] == "true"
	case "steps.tags.outputs.latest == 'true' && env.DISPATCH_TOKEN != ''":
		return r.outputs["tags"]["latest"] == "true" && r.job["DISPATCH_TOKEN"] != ""
	case "env.DISPATCH_TOKEN != ''":
		return r.job["DISPATCH_TOKEN"] != ""
	}
	r.t.Fatalf("the step condition %q is new to the dry run: teach the dry run this condition", cond)
	return false
}

// expr is a ${{ ... }} expression in a step's env.
var expr = regexp.MustCompile(`\$\{\{[^}]*\}\}`)

// subst replaces the ${{ ... }} expressions of a step's env values. Only the
// expressions this workflow uses are known; another one fails the test.
func (r *publishRun) subst(v string) string {
	r.t.Helper()
	var unknown string
	out := expr.ReplaceAllStringFunc(v, func(e string) string {
		switch name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(e, "${{"), "}}")); name {
		case "github.ref_name":
			return r.tag
		case "env.IMAGE":
			return publishImage
		case "steps.push.outputs.digest":
			return r.outputs["push"]["digest"]
		case "secrets.GITHUB_TOKEN":
			return "token"
		case "github.actor":
			return "actor"
		case "env.DISPATCH_TOKEN":
			return r.job["DISPATCH_TOKEN"]
		default:
			unknown = name
			return e
		}
	})
	if unknown != "" {
		r.t.Fatalf("the expression %q is new to the dry run: teach the dry run this expression", unknown)
	}
	return out
}

// env is the environment of one step: the job's, the step's, and its own
// GITHUB_OUTPUT file.
func (r *publishRun) env(s publishStep, output string) []string {
	r.t.Helper()
	env := []string{}
	for k, v := range r.job {
		env = append(env, k+"="+v)
	}
	for k, v := range s.Env {
		env = append(env, k+"="+r.subst(v))
	}
	return append(env, "GITHUB_OUTPUT="+output)
}

// readOutput records the key=value lines a step wrote to its GITHUB_OUTPUT.
func (r *publishRun) readOutput(id, output string) {
	r.t.Helper()
	if id == "" {
		return
	}
	got := map[string]string{}
	data, err := os.ReadFile(output)
	if err != nil && !os.IsNotExist(err) {
		r.t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			got[k] = v
		}
	}
	r.outputs[id] = got
}

// commands is what the stubs logged, in order.
func (r *publishRun) commands() []string {
	r.t.Helper()
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(readFile(r.t, r.root, "log")), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// publishCommands is the log of a publish of tag that passed: the login, the
// version tag, the sha- tag and, with latest, the latest tag.
func publishCommands(tag string, latest bool) []string {
	out := []string{
		"skopeo login ghcr.io --username actor --password-stdin",
		"skopeo copy --digestfile digest oci:frostmail.oci docker://" + publishImage + ":" + tag,
		"skopeo copy --preserve-digests docker://" + publishImage + "@" + publishDigest +
			" docker://" + publishImage + ":sha-" + publishSHA[:12],
	}
	if latest {
		out = append(out, "skopeo copy --preserve-digests docker://"+publishImage+"@"+publishDigest+
			" docker://"+publishImage+":latest")
	}
	return out
}

func TestFlatpakPublishMovesLatestOnlyForARelease(t *testing.T) {
	steps := publishSteps(t)
	dispatch := "gh workflow run publish.yml --repo frostyard/flatpak-index"
	for _, tc := range []struct {
		name  string
		tag   string
		token string
		want  []string
	}{
		{name: "a release", tag: "v0.4.0", want: publishCommands("v0.4.0", true)},
		{name: "a release and the dispatch token", tag: "v0.4.0", token: "x",
			want: append(publishCommands("v0.4.0", true), dispatch)},
		{name: "a release candidate", tag: "v0.4.0-rc.1", token: "x",
			want: publishCommands("v0.4.0-rc.1", false)},
		{name: "a later release candidate", tag: "v0.4.0-rc.12",
			want: publishCommands("v0.4.0-rc.12", false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPublishRun(t, tc.tag, tc.token)
			if fail, out := r.run(steps); fail != "" {
				t.Fatalf("step %q failed:\n%s", fail, out)
			}
			// The exact log: a release candidate has no latest copy at all.
			if got := r.commands(); !slices.Equal(got, tc.want) {
				t.Fatalf("tag %s ran\n%s\nwant\n%s", tc.tag, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
			if r.attested != publishDigest {
				t.Fatalf("tag %s attested %q, want the pushed digest %q", tc.tag, r.attested, publishDigest)
			}
		})
	}
}

func TestFlatpakPublishRefusesOtherTags(t *testing.T) {
	steps := publishSteps(t)
	for _, tag := range []string{"v0.4", "v0.4.0-beta.1", "v0.4.0-rc", "v0.4.0-rc.1-x", "vnext", "v0.4.0.1"} {
		t.Run(tag, func(t *testing.T) {
			r := newPublishRun(t, tag, "x")
			fail, out := r.run(steps)
			if fail != "Name the OCI tags" {
				t.Fatalf("tag %s failed at %q, want %q:\n%s", tag, fail, "Name the OCI tags", out)
			}
			if cmds := r.commands(); len(cmds) != 0 {
				t.Fatalf("tag %s published:\n%s", tag, strings.Join(cmds, "\n"))
			}
			if r.attested != "" {
				t.Fatalf("tag %s attested %q, want no attestation", tag, r.attested)
			}
		})
	}
}

func TestFlatpakPublishChecksTheTagFirstAndAttestsBeforeLatest(t *testing.T) {
	steps := publishSteps(t)
	if len(steps) == 0 {
		t.Fatal("the publish job has no steps")
	}
	if steps[0].Name != "Name the OCI tags" {
		t.Fatalf("the first step of the publish job is %q, want the tag check", steps[0].label())
	}
	index := func(name string) int {
		t.Helper()
		if i := slices.IndexFunc(steps, func(s publishStep) bool { return s.label() == name }); i >= 0 {
			return i
		}
		t.Fatalf("the publish job has no step %q", name)
		return -1
	}
	attest := slices.IndexFunc(steps, func(s publishStep) bool {
		return strings.Contains(s.Uses, "attest-build-provenance")
	})
	if attest < 0 {
		t.Fatal("the publish job never attests the image")
	}
	push, latest := index("Push the version to GHCR"), index("Move latest to the attested digest")
	if push >= attest || attest >= latest {
		t.Fatalf("the order is push %d, attest %d, latest %d: the attestation must come between them", push, attest, latest)
	}
	// The actions' order, as the dry run meets it: the tag check first, then
	// the artifact it publishes.
	r := newPublishRun(t, "v0.4.0", "")
	if fail, out := r.run(steps); fail != "" {
		t.Fatalf("step %q failed:\n%s", fail, out)
	}
	if r.order[0] != "Name the OCI tags" {
		t.Fatalf("the dry run met %q first, want the tag check", r.order[0])
	}
	if download := slices.IndexFunc(steps, func(s publishStep) bool { return strings.Contains(s.Uses, "download-artifact") }); download < 0 {
		t.Fatal("the publish job never downloads the OCI image")
	} else if download < index("Name the OCI tags") {
		t.Fatal("the download comes before the tag check")
	}
}

func TestReleaseWorkflowSkipsCandidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			Push struct {
				Tags []string `yaml:"tags"`
			} `yaml:"push"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("release.yml: %v", err)
	}
	patterns := wf.On.Push.Tags
	if len(patterns) == 0 {
		t.Fatal("release.yml has no on.push.tags")
	}
	for _, tc := range []struct {
		tag  string
		want bool
	}{
		{"v0.4.0", true},
		{"v1.2.3", true},
		// A candidate publishes the Flatpak only: no .deb and no release.
		{"v0.4.0-rc.1", false},
	} {
		if got := tagFilter(patterns, tc.tag); got != tc.want {
			t.Fatalf("release.yml tags %q with %v, want %v", tc.tag, patterns, tc.want)
		}
	}
}

// tagFilter reports whether a GitHub tag filter matches a tag: the patterns in
// order, a leading ! excludes, and * is any run of characters but /.
func tagFilter(patterns []string, tag string) bool {
	matched := false
	for _, p := range patterns {
		if rest, ok := strings.CutPrefix(p, "!"); ok {
			if tagGlob(rest, tag) {
				matched = false
			}
			continue
		}
		if tagGlob(p, tag) {
			matched = true
		}
	}
	return matched
}

// tagGlob reports whether one filter pattern matches tag, with * as any run
// of characters but /.
func tagGlob(pattern, tag string) bool {
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, "[^/]*") + "$"
	ok, err := regexp.MatchString(re, tag)
	if err != nil {
		panic(err)
	}
	return ok
}
