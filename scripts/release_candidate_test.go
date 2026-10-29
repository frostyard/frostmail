//go:build linux

package scripts_test

// The tests of release.yml's candidate job (ADR-0048) are a dry run of that
// job, in the way of flatpak_promote_test.go: they parse the job's step, run
// it in a temp work dir, and play gh with a stub that logs the command it is
// given and answers the commits API and the matching-refs API from files. So
// they check that release.yml builds the .deb of a tested candidate's commit
// only, refuses a tag on any other commit, and hands the commit to the build
// job, without a token and without a release.

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	// releaseRepository is github.repository, as the test sets it.
	releaseRepository = "frostyard/frostmail"
	// The release tag the cases push, and its candidates.
	releaseVersion = "v0.4.0"
	releaseRC1     = "v0.4.0-rc.1"
	releaseRC2     = "v0.4.0-rc.2"
	// The commits the cases play: one for the first candidate, one for the
	// second, and a later commit that no candidate names.
	releaseCommitRC1   = "1111111111111111111111111111111111111111"
	releaseCommitRC2   = "2222222222222222222222222222222222222222"
	releaseCommitLater = "3333333333333333333333333333333333333333"
	// The names of the steps the cases check.
	stepCandidateTag = "Check the tag is a candidate's commit"
	stepCheckout     = "actions/checkout"
	stepCheckoutHead = "The checkout is the candidate's commit"
	releaseBuildJob  = "build"
	releaseCheckJob  = "candidate"
	releaseCheckName = "The tag is a tested candidate's commit"
)

// releaseStubs is the stub of gh: it logs "gh args..." as one line, plays
// repos/<repo>/commits/<name> with the sha in RELEASE_STATE/commits/<name>,
// and plays repos/<repo>/git/matching-refs/tags/<prefix> with the files of
// that dir whose name starts with the prefix.
const releaseStubs = `#!/bin/bash
set -u
{ printf 'gh'; [ 0 -eq $# ] || printf ' %s' "$@"; printf '\n'; } >>"$RELEASE_LOG"
state=$RELEASE_STATE
if [ "${1:-}" != api ]; then
	echo "gh: unknown command $*" >&2
	exit 1
fi
shift
jq= prev= endpoint=
for arg in "$@"; do
	if [ "$prev" = --jq ]; then
		jq=$arg
		prev=$arg
		continue
	fi
	case "$arg" in
	--paginate | --jq) ;;
	-*) ;;
	*) [ -n "$endpoint" ] || endpoint=$arg ;;
	esac
	prev=$arg
done
case "$endpoint" in
repos/*/commits/*)
	name=${endpoint##*/commits/}
	if [ ! -f "$state/commits/$name" ]; then
		echo "gh: Not Found (HTTP 404)" >&2
		exit 1
	fi
	sha=$(cat "$state/commits/$name")
	if [ "$jq" = .sha ]; then printf '%s\n' "$sha"; else printf '{"sha":"%s"}\n' "$sha"; fi
	;;
repos/*/git/matching-refs/tags/*)
	prefix=${endpoint##*/matching-refs/tags/}
	refs=
	for f in "$state"/commits/*; do
		[ -f "$f" ] || continue
		name=$(basename "$f")
		case "$name" in "$prefix"*) ;; *) continue ;; esac
		refs="$refs refs/tags/$name"
	done
	if [ "$jq" = '.[].ref' ]; then
		for ref in $refs; do printf '%s\n' "$ref"; done
	else
		json=
		for ref in $refs; do json="$json${json:+,}{\"ref\":\"$ref\"}"; done
		printf '[%s]\n' "$json"
	fi
	;;
*)
	echo "gh: unexpected endpoint $endpoint" >&2
	exit 1
	;;
esac
`

// stringList is a YAML scalar or sequence, as a job's needs may be either.
type stringList []string

func (s *stringList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*s = []string{value.Value}
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}

// releaseJob is one job of release.yml, as the workflow writes it.
type releaseJob struct {
	Name           string            `yaml:"name"`
	Needs          stringList        `yaml:"needs"`
	RunsOn         string            `yaml:"runs-on"`
	TimeoutMinutes int               `yaml:"timeout-minutes"`
	Permissions    map[string]string `yaml:"permissions"`
	Outputs        map[string]string `yaml:"outputs"`
	Env            map[string]string `yaml:"env"`
	Steps          []publishStep     `yaml:"steps"`
}

// releaseJobs reads release.yml and returns its jobs by name.
func releaseJobs(t *testing.T) map[string]releaseJob {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]releaseJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("release.yml: %v", err)
	}
	for _, name := range []string{releaseCheckJob, releaseBuildJob} {
		if len(wf.Jobs[name].Steps) == 0 {
			t.Fatalf("release.yml has no %s job with steps", name)
		}
	}
	return wf.Jobs
}

// releaseRun is one dry run of release.yml's candidate job, for one tag.
type releaseRun struct {
	t       *testing.T
	root    string
	work    string
	state   string
	log     string
	tag     string
	outputs map[string]string
}

// newReleaseRun stages a work dir, a state dir and the stub gh for tag.
func newReleaseRun(t *testing.T, tag string) *releaseRun {
	t.Helper()
	root := t.TempDir()
	r := &releaseRun{
		t:       t,
		root:    root,
		work:    filepath.Join(root, "work"),
		state:   filepath.Join(root, "state"),
		log:     filepath.Join(root, "log"),
		tag:     tag,
		outputs: map[string]string{},
	}
	for _, dir := range []string{r.work, filepath.Join(r.state, "commits")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, root, "log", "")
	writeFile(t, root, "bin/gh", releaseStubs)
	if err := os.Chmod(filepath.Join(root, "bin", "gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

// setCommit gives a tag or commit name the sha the commits API answers.
func (r *releaseRun) setCommit(name, sha string) {
	r.t.Helper()
	writeFile(r.t, r.state, "commits/"+name, sha)
}

// run runs the candidate job's steps in order and stops at the first that
// fails, as GitHub does. It returns the label and the output of the step that
// failed, or "" and the last step's output when every step passed.
func (r *releaseRun) run(job releaseJob) (string, string) {
	r.t.Helper()
	out, code := "", 0
	for _, s := range job.Steps {
		if s.Uses != "" {
			r.t.Fatalf("step %q is an action, and the candidate job has none", s.label())
		}
		output := filepath.Join(r.work, "github-output-"+s.ID)
		if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
			r.t.Fatal(err)
		}
		cmd := exec.CommandContext(r.t.Context(), "bash", "-e", "-c", s.Run)
		cmd.Dir = r.work
		cmd.Env = r.env(s, output)
		out, code = runScript(r.t, cmd)
		r.readOutput(s.ID, output)
		if code != 0 {
			return s.label(), out
		}
	}
	return "", out
}

// env is the environment of one step: the runner's, the step's and its own
// GITHUB_OUTPUT file.
func (r *releaseRun) env(s publishStep, output string) []string {
	r.t.Helper()
	env := []string{
		"PATH=" + filepath.Join(r.root, "bin") + ":" + os.Getenv("PATH"),
		"RELEASE_LOG=" + r.log,
		"RELEASE_STATE=" + r.state,
	}
	for k, v := range s.Env {
		env = append(env, k+"="+r.subst(v))
	}
	return append(env, "GITHUB_OUTPUT="+output)
}

// subst replaces the ${{ ... }} expressions of an env value. Only the
// expressions this job uses are known; another one fails the test.
func (r *releaseRun) subst(v string) string {
	r.t.Helper()
	var unknown string
	out := expr.ReplaceAllStringFunc(v, func(e string) string {
		switch name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(e, "${{"), "}}")); name {
		case "github.repository":
			return releaseRepository
		case "github.ref_name":
			return r.tag
		case "github.token":
			return "token"
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

// readOutput records the key=value lines the step wrote to its GITHUB_OUTPUT.
func (r *releaseRun) readOutput(id, output string) {
	r.t.Helper()
	if id == "" {
		return
	}
	data, err := os.ReadFile(output)
	if err != nil && !os.IsNotExist(err) {
		r.t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			r.outputs[k] = v
		}
	}
}

// commands is what the stub logged, in order.
func (r *releaseRun) commands() []string {
	r.t.Helper()
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(readFile(r.t, r.root, "log")), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestReleaseCandidatePassesACandidatesCommit checks that a release tag on the
// commit of one of its candidates passes, and names that commit and candidate.
func TestReleaseCandidatePassesACandidatesCommit(t *testing.T) {
	jobs := releaseJobs(t)
	for _, tc := range []struct {
		name, sha, want string
	}{
		// The release tag is on the first candidate's commit.
		{name: "the first candidate", sha: releaseCommitRC1, want: releaseRC1},
		// The gate passed a later candidate: the tag is on that commit.
		{name: "the second candidate", sha: releaseCommitRC2, want: releaseRC2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReleaseRun(t, releaseVersion)
			r.setCommit(releaseVersion, tc.sha)
			r.setCommit(releaseRC1, releaseCommitRC1)
			r.setCommit(releaseRC2, releaseCommitRC2)
			fail, out := r.run(jobs[releaseCheckJob])
			if fail != "" {
				t.Fatalf("step %q failed:\n%s", fail, out)
			}
			if got := r.outputs["commit"]; got != tc.sha {
				t.Fatalf("the job output commit is %q, want the candidate's %q", got, tc.sha)
			}
			if got := r.outputs["candidate"]; got != tc.want {
				t.Fatalf("the job output candidate is %q, want %q", got, tc.want)
			}
			if want := "::notice::" + releaseVersion + " is the commit of the candidate " + tc.want; !strings.Contains(out, want) {
				t.Fatalf("the step printed\n%s\nwant %q", out, want)
			}
			// It reads the release tag's commit, which the outputs name.
			if cmds := r.commands(); !slices.Contains(cmds,
				"gh api repos/"+releaseRepository+"/commits/"+releaseVersion+" --jq .sha") {
				t.Fatalf("the job never read the release tag's commit:\n%s", strings.Join(cmds, "\n"))
			}
		})
	}
}

// TestReleaseCandidateRefusesATagOnAnotherCommit checks the refusals: a tag on
// a commit that no candidate of its version names builds no .deb.
func TestReleaseCandidateRefusesATagOnAnotherCommit(t *testing.T) {
	jobs := releaseJobs(t)
	for _, tc := range []struct {
		name    string
		tag     string
		commits map[string]string
	}{
		// A later commit of main: no candidate names it.
		{name: "a later commit", tag: releaseVersion, commits: map[string]string{
			releaseVersion: releaseCommitLater,
			releaseRC1:     releaseCommitRC1,
			releaseRC2:     releaseCommitRC2,
		}},
		// A candidate of another version on the same commit does not count.
		{name: "another version's candidate", tag: releaseVersion, commits: map[string]string{
			releaseVersion: releaseCommitRC1,
			"v0.4.1-rc.1":  releaseCommitRC1,
		}},
		// No candidate at all.
		{name: "no candidates", tag: releaseVersion, commits: map[string]string{
			releaseVersion: releaseCommitRC1,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReleaseRun(t, tc.tag)
			for name, sha := range tc.commits {
				r.setCommit(name, sha)
			}
			fail, out := r.run(jobs[releaseCheckJob])
			if fail != stepCandidateTag {
				t.Fatalf("the run failed at %q, want %q:\n%s", fail, stepCandidateTag, out)
			}
			if want := tc.tag + " must be the commit of a tested candidate " + tc.tag + "-rc.N"; !strings.Contains(out, want) {
				t.Fatalf("the step printed\n%s\nwant %q", out, want)
			}
			if len(r.outputs) != 0 {
				t.Fatalf("the refused tag wrote the outputs %v", r.outputs)
			}
		})
	}
}

// TestReleaseCandidateRefusesATagThatIsNotARelease checks that a candidate tag
// itself is refused before anything is read.
func TestReleaseCandidateRefusesATagThatIsNotARelease(t *testing.T) {
	jobs := releaseJobs(t)
	for _, tag := range []string{releaseRC1, "v0.4", "v0.4.0-beta.1", "vnext", "v0.4.0.1"} {
		t.Run(tag, func(t *testing.T) {
			r := newReleaseRun(t, tag)
			r.setCommit(tag, releaseCommitRC1)
			fail, out := r.run(jobs[releaseCheckJob])
			if fail != stepCandidateTag {
				t.Fatalf("tag %s failed at %q, want %q:\n%s", tag, fail, stepCandidateTag, out)
			}
			if cmds := r.commands(); len(cmds) != 0 {
				t.Fatalf("tag %s ran\n%s", tag, strings.Join(cmds, "\n"))
			}
		})
	}
}

// TestReleaseBuildsTheCandidateCommit checks the shape of release.yml: the
// candidate job's single check, its outputs, and the build job's use of them.
func TestReleaseBuildsTheCandidateCommit(t *testing.T) {
	jobs := releaseJobs(t)
	check := jobs[releaseCheckJob]
	if check.Name != releaseCheckName {
		t.Fatalf("the candidate job is named %q, want %q", check.Name, releaseCheckName)
	}
	if check.RunsOn != "ubuntu-24.04" {
		t.Fatalf("the candidate job runs on %q, want ubuntu-24.04", check.RunsOn)
	}
	if check.TimeoutMinutes != 5 {
		t.Fatalf("the candidate job's timeout is %d minutes, want 5", check.TimeoutMinutes)
	}
	if got := check.Permissions["contents"]; got != "read" {
		t.Fatalf("the candidate job's contents permission is %q, want read", got)
	}
	if len(check.Steps) != 1 {
		t.Fatalf("the candidate job has %d steps, want the tag check alone", len(check.Steps))
	}
	step := check.Steps[0]
	if step.label() != stepCandidateTag || step.ID != "check" {
		t.Fatalf("the candidate job's step is %q id %q, want %q id check", step.label(), step.ID, stepCandidateTag)
	}
	for _, name := range []string{"GH_TOKEN", "REPO", "TAG"} {
		if step.Env[name] == "" {
			t.Fatalf("the tag check has no env %s: the inputs reach it through the environment only", name)
		}
	}
	// The inputs reach the script through env only: an expression in run:
	// would let a tag inject a command.
	if strings.Contains(step.Run, "${{") {
		t.Fatalf("the tag check's run script holds an expression:\n%s", step.Run)
	}
	if got, want := check.Outputs["commit"], "${{ steps.check.outputs.commit }}"; got != want {
		t.Fatalf("the candidate job's commit output is %q, want %q", got, want)
	}

	build := jobs[releaseBuildJob]
	if !slices.Contains(build.Needs, releaseCheckJob) {
		t.Fatalf("the build job needs %v, want %s", build.Needs, releaseCheckJob)
	}
	index := func(name string) int {
		t.Helper()
		if i := slices.IndexFunc(build.Steps, func(s publishStep) bool { return s.label() == name }); i >= 0 {
			return i
		}
		t.Fatalf("the build job has no step %q", name)
		return -1
	}
	checkout := slices.IndexFunc(build.Steps, func(s publishStep) bool {
		return strings.Contains(s.Uses, stepCheckout)
	})
	if checkout < 0 {
		t.Fatal("the build job never checks the tag out")
	}
	if got := index(stepCheckoutHead); got != checkout+1 {
		t.Fatalf("the checkout check is step %d, want %d, right after the checkout", got, checkout+1)
	}
	if got, want := build.Steps[index(stepCheckoutHead)].Env["COMMIT"], "${{ needs.candidate.outputs.commit }}"; got != want {
		t.Fatalf("the checkout check compares against %q, want %q", got, want)
	}
}

// TestReleasePublishesNoFlatpak checks that release.yml holds no Flatpak and
// no registry: the .deb and the GitHub release are all it makes.
func TestReleasePublishesNoFlatpak(t *testing.T) {
	for name, job := range releaseJobs(t) {
		for _, s := range job.Steps {
			texts := []string{s.label(), s.If, s.Run}
			for k, v := range s.With {
				texts = append(texts, k+"="+v)
			}
			for k, v := range s.Env {
				texts = append(texts, k+"="+v)
			}
			for _, text := range texts {
				for _, banned := range []string{"flatpak", "skopeo", "ghcr.io"} {
					if strings.Contains(strings.ToLower(text), banned) {
						t.Fatalf("step %q of the %s job names %s", s.label(), name, banned)
					}
				}
			}
		}
	}
}
