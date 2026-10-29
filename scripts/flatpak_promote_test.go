//go:build linux

package scripts_test

// The tests of the promotion workflow (.github/workflows/promote.yml, plan
// 0013 Phases 8 and 10) are a dry run of its promote job, in the way of
// flatpak_publish_test.go: they parse the job's steps, run them in order in a
// temp work dir, and play skopeo, gh and docker with stubs that log the command
// they are given. So they check that a promotion copies the candidate's digest
// to the version tag and to latest, builds and attests nothing, and refuses
// what it must, without a registry, a token or a Flatpak build.

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	// promoteRepository is github.repository, as the test sets it.
	promoteRepository = "frostyard/frostmail"
	promoteImage      = publishImage
	// The candidate the passing cases promote, and its release.
	promoteCandidate = "v0.4.0-rc.1"
	promoteVersion   = "v0.4.0"
	// The digest the candidate's tag holds: sha256: and 64 b's.
	promoteDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// The digest of another release: sha256: and 64 c's.
	promoteOtherDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	// The workflow file whose attestation counts, and the one that does not.
	promoteSigner = ".github/workflows/flatpak.yml"
	otherSigner   = ".github/workflows/release.yml"
	// The names of the steps the cases check.
	stepInputs  = "Check the inputs"
	stepDigest  = "Resolve the candidate's digest"
	stepCompare = "Check the release tag is the candidate's commit"
	stepVerify  = "Verify the candidate's attestation"
	stepFree    = "Check the version tag is free"
	stepVersion = "Copy the candidate to the version"
	stepLatest  = "Move latest to the candidate"
)

// promoteStubs is the stub of skopeo, gh and docker: it logs "name args..." as
// one line, and plays the registry, the compare API and the attestation from
// the files in PROMOTE_STATE. tags/<tag> holds the digest of an OCI tag,
// compare holds the compare status, and attestation.json is what
// gh attestation verify prints.
const promoteStubs = `#!/bin/bash
set -u
name=$(basename "$0")
{ printf '%s' "$name"; [ 0 -eq $# ] || printf ' %s' "$@"; printf '\n'; } >>"$PROMOTE_LOG"
state=$PROMOTE_STATE
case "$name" in
skopeo)
	case "${1:-}" in
	login)
		cat >/dev/null
		;;
	inspect)
		image=${!#}
		tag=${image##*:}
		if [ ! -f "$state/tags/$tag" ]; then
			echo "manifest unknown: $image" >&2
			exit 1
		fi
		cat "$state/tags/$tag"
		printf '\n'
		;;
	list-tags)
		list=
		for f in "$state"/tags/*; do
			[ -f "$f" ] || continue
			[ -z "$list" ] || list="$list,"
			list="$list\"$(basename "$f")\""
		done
		printf '{"Repository":"%s","Tags":[%s]}\n' "$IMAGE" "$list"
		;;
	copy)
		src=${@: -2:1}
		dst=${!#}
		printf '%s' "${src##*@}" >"$state/tags/${dst##*:}"
		;;
	esac
	;;
gh)
	if [ "${1:-}" = api ]; then
		status=$(cat "$state/compare" 2>/dev/null) || {
			echo "gh: Not Found (HTTP 404)" >&2
			exit 1
		}
		jq= prev=
		for arg in "$@"; do
			if [ "$prev" = --jq ]; then jq=$arg; fi
			prev=$arg
		done
		if [ "$jq" = .status ]; then printf '%s\n' "$status"; else printf '{"status":"%s"}\n' "$status"; fi
	elif [ "${1:-}" = attestation ]; then
		if [ ! -f "$state/attestation.json" ]; then
			echo "gh: no attestation for the subject" >&2
			exit 1
		fi
		cat "$state/attestation.json"
		printf '\n'
	elif [ "${1:-}" = workflow ]; then
		: # gh workflow run: logged only
	fi
	;;
docker)
	cat >/dev/null
	;;
esac
`

// promoteWorkflow is promote.yml's promote job, as the workflow writes it.
type promoteWorkflow struct {
	Permissions map[string]string `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`
	Steps       []publishStep     `yaml:"steps"`
}

// promoteJob reads promote.yml and checks its trigger, then returns its
// promote job's permissions, env and steps.
func promoteJob(t *testing.T) promoteWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "promote.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			WorkflowDispatch struct {
				Inputs map[string]struct {
					Description string `yaml:"description"`
					Required    bool   `yaml:"required"`
					Type        string `yaml:"type"`
				} `yaml:"inputs"`
			} `yaml:"workflow_dispatch"`
		} `yaml:"on"`
		Jobs map[string]struct {
			Name        string            `yaml:"name"`
			RunsOn      string            `yaml:"runs-on"`
			Permissions map[string]string `yaml:"permissions"`
			Env         map[string]string `yaml:"env"`
			Steps       []publishStep     `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("promote.yml: %v", err)
	}
	job, ok := wf.Jobs["promote"]
	if !ok || len(job.Steps) == 0 {
		t.Fatal("promote.yml has no promote job with steps")
	}
	// The trigger and its inputs, which the run and its cases need.
	inputs := wf.On.WorkflowDispatch.Inputs
	if len(inputs) != 2 {
		t.Fatalf("promote.yml has %d workflow_dispatch inputs, want candidate and version", len(inputs))
	}
	for _, name := range []string{"candidate", "version"} {
		in, ok := inputs[name]
		if !ok {
			t.Fatalf("promote.yml has no workflow_dispatch input %q", name)
		}
		if !in.Required || in.Type != "string" {
			t.Fatalf("the input %q is required=%v type=%q, want a required string", name, in.Required, in.Type)
		}
	}
	if job.RunsOn != "ubuntu-latest" {
		t.Fatalf("the promote job runs on %q, want ubuntu-latest: flatpak-index rejects a self-hosted runner's attestation", job.RunsOn)
	}
	return promoteWorkflow{Permissions: job.Permissions, Env: job.Env, Steps: job.Steps}
}

// promoteRun is one dry run of the promote job, for one candidate, version and
// dispatch token.
type promoteRun struct {
	t                         *testing.T
	root, work, state, log    string
	candidate, version, token string
	steps                     int
	// env is the job's env, substituted for this run.
	env     map[string]string
	outputs map[string]map[string]string
	// order is the label of every step the run met, run or skipped.
	order []string
}

// newPromoteRun stages a work dir, a state dir and the stub commands.
func newPromoteRun(t *testing.T, wf promoteWorkflow, candidate, version, token string) *promoteRun {
	t.Helper()
	root := t.TempDir()
	r := &promoteRun{
		t:         t,
		root:      root,
		work:      filepath.Join(root, "work"),
		state:     filepath.Join(root, "state"),
		log:       filepath.Join(root, "log"),
		candidate: candidate,
		version:   version,
		token:     token,
		env:       map[string]string{},
		outputs:   map[string]map[string]string{},
	}
	for k, v := range wf.Env {
		r.env[k] = r.subst(v)
	}
	if err := os.MkdirAll(r.work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(r.state, "tags"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "log", "")
	writeFile(t, root, "bin/stub", promoteStubs)
	if err := os.Chmod(filepath.Join(root, "bin", "stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skopeo", "gh", "docker"} {
		if err := os.Symlink("stub", filepath.Join(root, "bin", name)); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// setTag gives an OCI tag a digest in the stub registry.
func (r *promoteRun) setTag(tag, digest string) {
	r.t.Helper()
	writeFile(r.t, r.state, "tags/"+tag, digest)
}

// setCompare gives the compare API a status.
func (r *promoteRun) setCompare(status string) {
	r.t.Helper()
	writeFile(r.t, r.state, "compare", status)
}

// setAttestation gives gh attestation verify output, in the shape it prints.
func (r *promoteRun) setAttestation(items ...attestationItem) {
	r.t.Helper()
	var parts []string
	for _, item := range items {
		parts = append(parts, `{"verificationResult":{"signature":{"certificate":{"sourceRepositoryRef":"`+
			item.ref+`","buildSignerURI":"`+item.signer+`"}}}}`)
	}
	writeFile(r.t, r.state, "attestation.json", "["+strings.Join(parts, ",")+"]")
}

// attestationItem is one attestation that gh attestation verify reports.
type attestationItem struct {
	ref    string
	signer string
}

// goodAttestation is the candidate's attestation: flatpak.yml, on the
// candidate's tag.
func goodAttestation(candidate string) attestationItem {
	return attestationItem{
		ref:    "refs/tags/" + candidate,
		signer: "https://github.com/" + promoteRepository + "/" + promoteSigner + "@refs/tags/" + candidate,
	}
}

// run runs the steps in order and stops at the first that fails, as GitHub
// does. It returns the label and the output of the step that failed, or "",
// when every step passed.
func (r *promoteRun) run(steps []publishStep) (string, string) {
	r.t.Helper()
	for _, s := range steps {
		r.order = append(r.order, s.label())
		if s.Uses != "" {
			r.t.Fatalf("step %q is an action, and the promote job has none", s.label())
		}
		if !r.condition(s.If) {
			continue
		}
		output := filepath.Join(r.work, "github-output-"+r.slot(s))
		if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
			r.t.Fatal(err)
		}
		cmd := exec.CommandContext(r.t.Context(), "bash", "-e", "-c", s.Run)
		cmd.Dir = r.work
		cmd.Env = r.stepEnv(s, output)
		out, code := runScript(r.t, cmd)
		r.readOutput(s.ID, output)
		if code != 0 {
			return s.label(), out
		}
	}
	return "", ""
}

// slot is a file name piece for a step's GITHUB_OUTPUT, since a step may have
// no id.
func (r *promoteRun) slot(s publishStep) string {
	if s.ID != "" {
		return s.ID
	}
	r.steps++
	return "step" + strconv.Itoa(r.steps)
}

// condition evaluates a step's if. It knows the conditions of this workflow
// and fails the test on any other, so a new one is taught to the dry run.
func (r *promoteRun) condition(cond string) bool {
	r.t.Helper()
	switch strings.TrimSpace(cond) {
	case "":
		return true
	case "env.DISPATCH_TOKEN != ''":
		return r.token != ""
	}
	r.t.Fatalf("the step condition %q is new to the dry run: teach the dry run this condition", cond)
	return false
}

// subst replaces the ${{ ... }} expressions of an env value. Only the
// expressions this workflow uses are known; another one fails the test.
func (r *promoteRun) subst(v string) string {
	r.t.Helper()
	var unknown string
	out := expr.ReplaceAllStringFunc(v, func(e string) string {
		switch name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(e, "${{"), "}}")); name {
		case "github.repository":
			return promoteRepository
		case "inputs.candidate":
			return r.candidate
		case "inputs.version":
			return r.version
		case "secrets.FLATPAK_INDEX_DISPATCH_TOKEN", "env.DISPATCH_TOKEN":
			return r.token
		case "secrets.GITHUB_TOKEN", "github.token":
			return "token"
		case "github.actor":
			return "actor"
		case "env.IMAGE":
			return promoteImage
		case "steps.candidate.outputs.digest":
			return r.outputs["candidate"]["digest"]
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

// stepEnv is the environment of one step: the job's env, the step's, and its
// own GITHUB_OUTPUT file.
func (r *promoteRun) stepEnv(s publishStep, output string) []string {
	r.t.Helper()
	env := []string{
		"PATH=" + filepath.Join(r.root, "bin") + ":" + os.Getenv("PATH"),
		"PROMOTE_LOG=" + r.log,
		"PROMOTE_STATE=" + r.state,
	}
	for k, v := range r.env {
		env = append(env, k+"="+v)
	}
	for k, v := range s.Env {
		env = append(env, k+"="+r.subst(v))
	}
	return append(env, "GITHUB_OUTPUT="+output)
}

// readOutput records the key=value lines a step wrote to its GITHUB_OUTPUT.
func (r *promoteRun) readOutput(id, output string) {
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
func (r *promoteRun) commands() []string {
	r.t.Helper()
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(readFile(r.t, r.root, "log")), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// wantLog checks the exact command log.
func (r *promoteRun) wantLog(want []string) {
	r.t.Helper()
	if got := r.commands(); !slices.Equal(got, want) {
		r.t.Fatalf("the stubs ran\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// noCopy fails the test when the stubs copied an image.
func (r *promoteRun) noCopy() {
	r.t.Helper()
	for _, cmd := range r.commands() {
		if strings.Contains(cmd, "skopeo copy") {
			r.t.Fatalf("the run copied an image: %s", cmd)
		}
	}
}

// copyCommand is the copy of the candidate's digest to tag.
func copyCommand(tag string) string {
	return "skopeo copy --preserve-digests docker://" + promoteImage + "@" + promoteDigest +
		" docker://" + promoteImage + ":" + tag
}

// verifyCommand is the attestation check of the candidate's digest, as the
// workflow writes it.
func verifyCommand(candidate string) string {
	return "gh attestation verify oci://" + promoteImage + "@" + promoteDigest +
		" --repo " + promoteRepository +
		" --signer-workflow github.com/" + promoteRepository + "/" + promoteSigner +
		" --source-ref refs/tags/" + candidate +
		" --deny-self-hosted-runners --format json"
}

// promoteLog is the log of a promotion that goes to the end: the logins, the
// candidate's digest, the compare, the attestation check, the tag list, the
// two copies and, with a dispatch token, the dispatch. versionExists adds the
// read of a version tag that is already there.
func promoteLog(candidate, version string, versionExists, dispatch bool) []string {
	out := []string{
		"skopeo login ghcr.io --username actor --password-stdin",
		"docker login ghcr.io --username actor --password-stdin",
		"skopeo inspect --format {{.Digest}} docker://" + promoteImage + ":" + candidate,
		"gh api repos/" + promoteRepository + "/compare/" + candidate + "..." + version + " --jq .status",
		verifyCommand(candidate),
		"skopeo list-tags docker://" + promoteImage,
	}
	if versionExists {
		out = append(out, "skopeo inspect --format {{.Digest}} docker://"+promoteImage+":"+version)
	}
	out = append(out, copyCommand(version), copyCommand("latest"))
	if dispatch {
		out = append(out, "gh workflow run publish.yml --repo frostyard/flatpak-index")
	}
	return out
}

// ready stages a run whose candidate, release tag and attestation all pass.
func ready(t *testing.T, wf promoteWorkflow, token string) *promoteRun {
	t.Helper()
	r := newPromoteRun(t, wf, promoteCandidate, promoteVersion, token)
	r.setTag(promoteCandidate, promoteDigest)
	r.setCompare("identical")
	r.setAttestation(goodAttestation(promoteCandidate))
	return r
}

func TestFlatpakPromoteCopiesTheCandidateToTheRelease(t *testing.T) {
	wf := promoteJob(t)
	for _, tc := range []struct {
		name     string
		status   string
		dispatch string
	}{
		{name: "the candidate", status: "identical"},
		{name: "the candidate and the dispatch token", status: "identical", dispatch: "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPromoteRun(t, wf, promoteCandidate, promoteVersion, tc.dispatch)
			r.setTag(promoteCandidate, promoteDigest)
			r.setCompare(tc.status)
			r.setAttestation(goodAttestation(promoteCandidate))
			if fail, out := r.run(wf.Steps); fail != "" {
				t.Fatalf("step %q failed:\n%s", fail, out)
			}
			r.wantLog(promoteLog(promoteCandidate, promoteVersion, false, tc.dispatch != ""))
			// The tags now hold the candidate's digest.
			for _, tag := range []string{promoteVersion, "latest"} {
				if got := readFile(t, r.state, "tags/"+tag); got != promoteDigest {
					t.Fatalf("tag %s holds %q, want the candidate's %q", tag, got, promoteDigest)
				}
			}
			// The check flatpak-index makes, as the step writes it.
			for _, cmd := range r.commands() {
				if !strings.HasPrefix(cmd, "gh attestation verify") {
					continue
				}
				for _, want := range []string{
					"--signer-workflow github.com/frostyard/frostmail/" + promoteSigner,
					"--source-ref refs/tags/" + promoteCandidate,
					"--deny-self-hosted-runners",
				} {
					if !strings.Contains(cmd, want) {
						t.Fatalf("the check lacks %q: %s", want, cmd)
					}
				}
			}
		})
	}
}

func TestFlatpakPromoteAllowsTheVersionTagAtTheCandidateDigest(t *testing.T) {
	wf := promoteJob(t)
	r := ready(t, wf, "")
	r.setTag(promoteVersion, promoteDigest)
	if fail, out := r.run(wf.Steps); fail != "" {
		t.Fatalf("step %q failed:\n%s", fail, out)
	}
	// It reads the version tag, finds the candidate's digest and goes on.
	r.wantLog(promoteLog(promoteCandidate, promoteVersion, true, false))
}

func TestFlatpakPromoteRefusesAnotherDigestAtTheVersionTag(t *testing.T) {
	wf := promoteJob(t)
	r := ready(t, wf, "")
	r.setTag(promoteVersion, promoteOtherDigest)
	fail, out := r.run(wf.Steps)
	if fail != stepFree {
		t.Fatalf("the run failed at %q, want %q:\n%s", fail, stepFree, out)
	}
	r.noCopy()
	r.wantLog(promoteLog(promoteCandidate, promoteVersion, true, false)[:7])
}

func TestFlatpakPromoteRefusesAnUnverifiedCandidate(t *testing.T) {
	wf := promoteJob(t)
	for _, tc := range []struct {
		name  string
		items []attestationItem
	}{
		// gh exits 1: the digest has no attestation at all.
		{name: "no attestation"},
		// An attestation of another workflow file does not count.
		{name: "another workflow file", items: []attestationItem{{
			ref:    "refs/tags/" + promoteCandidate,
			signer: "https://github.com/" + promoteRepository + "/" + otherSigner + "@refs/tags/" + promoteCandidate,
		}}},
		// Nor does one of another tag, by the right workflow.
		{name: "another tag", items: []attestationItem{goodAttestation("v0.4.0-rc.2")}},
		// The two must be in one certificate, not one each.
		{name: "the ref and the signer apart", items: []attestationItem{
			{ref: "refs/tags/" + promoteCandidate,
				signer: "https://github.com/" + promoteRepository + "/" + otherSigner + "@refs/tags/" + promoteCandidate},
			goodAttestation("v0.4.0-rc.2"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPromoteRun(t, wf, promoteCandidate, promoteVersion, "")
			r.setTag(promoteCandidate, promoteDigest)
			r.setCompare("identical")
			if len(tc.items) > 0 {
				r.setAttestation(tc.items...)
			}
			fail, out := r.run(wf.Steps)
			if fail != stepVerify {
				t.Fatalf("the run failed at %q, want %q:\n%s", fail, stepVerify, out)
			}
			r.noCopy()
			r.wantLog(promoteLog(promoteCandidate, promoteVersion, false, false)[:5])
		})
	}
}

func TestFlatpakPromoteRefusesAReleaseTagThatIsNotTheCandidate(t *testing.T) {
	wf := promoteJob(t)
	for _, tc := range []struct {
		name   string
		status string
	}{
		// No compare file: the tag does not exist, so gh exits 1.
		{name: "no release tag"},
		// The release tag is on a later commit of main: the .deb would hold
		// other code than the candidate's Flatpak.
		{name: "ahead", status: "ahead"},
		{name: "behind", status: "behind"},
		{name: "diverged", status: "diverged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPromoteRun(t, wf, promoteCandidate, promoteVersion, "")
			r.setTag(promoteCandidate, promoteDigest)
			r.setAttestation(goodAttestation(promoteCandidate))
			if tc.status != "" {
				r.setCompare(tc.status)
			}
			fail, out := r.run(wf.Steps)
			if fail != stepCompare {
				t.Fatalf("the run failed at %q, want %q:\n%s", fail, stepCompare, out)
			}
			r.noCopy()
			r.wantLog(promoteLog(promoteCandidate, promoteVersion, false, false)[:4])
		})
	}
}

func TestFlatpakPromoteRefusesBadInputs(t *testing.T) {
	wf := promoteJob(t)
	for _, tc := range []struct {
		name               string
		candidate, version string
	}{
		{name: "a release as the candidate", candidate: promoteVersion, version: promoteVersion},
		{name: "a beta as the candidate", candidate: "v0.4.0-beta.1", version: promoteVersion},
		{name: "a short version", candidate: promoteCandidate, version: "v0.4"},
		{name: "another release", candidate: promoteCandidate, version: "v0.5.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPromoteRun(t, wf, tc.candidate, tc.version, "")
			fail, out := r.run(wf.Steps)
			if fail != stepInputs {
				t.Fatalf("the run failed at %q, want %q:\n%s", fail, stepInputs, out)
			}
			if cmds := r.commands(); len(cmds) != 0 {
				t.Fatalf("the refused inputs ran\n%s", strings.Join(cmds, "\n"))
			}
		})
	}
}

func TestFlatpakPromoteRefusesAMissingCandidate(t *testing.T) {
	wf := promoteJob(t)
	r := newPromoteRun(t, wf, promoteCandidate, promoteVersion, "")
	r.setCompare("identical")
	r.setAttestation(goodAttestation(promoteCandidate))
	fail, out := r.run(wf.Steps)
	if fail != stepDigest {
		t.Fatalf("the run failed at %q, want %q:\n%s", fail, stepDigest, out)
	}
	r.noCopy()
	r.wantLog(promoteLog(promoteCandidate, promoteVersion, false, false)[:3])
}

// TestFlatpakPromoteBuildsAndAttestsNothing checks the job's shape: no action,
// no build, no attestation of its own, no id-token, and the order of the
// checks and the copies.
func TestFlatpakPromoteBuildsAndAttestsNothing(t *testing.T) {
	wf := promoteJob(t)
	index := func(name string) int {
		t.Helper()
		if i := slices.IndexFunc(wf.Steps, func(s publishStep) bool { return s.label() == name }); i >= 0 {
			return i
		}
		t.Fatalf("the promote job has no step %q", name)
		return -1
	}
	if got, ok := wf.Permissions["id-token"]; ok {
		t.Fatalf("the job has id-token: %s, and it makes no attestation of its own", got)
	}
	if got := wf.Permissions["attestations"]; got != "read" {
		t.Fatalf("the job's attestations permission is %q, want read: it verifies and attests nothing", got)
	}
	for _, s := range wf.Steps {
		if s.Uses != "" {
			t.Fatalf("step %q uses an action; the promote job has none", s.label())
		}
		for _, banned := range []string{"flatpak-builder", "build-bundle", "attest-build-provenance", "attestation create"} {
			if strings.Contains(s.Run, banned) {
				t.Fatalf("step %q builds or attests (%s): only the candidate's attestation counts", s.label(), banned)
			}
		}
		// The one attestation word the job may hold is the check itself.
		if s.label() != stepVerify && strings.Contains(s.Run, "attest") {
			t.Fatalf("step %q names an attestation; only %q checks the candidate's", s.label(), stepVerify)
		}
	}
	verify, check, latest := index(stepVerify), index(stepVersion), index(stepLatest)
	script := wf.Steps[verify].Run
	for _, want := range []string{promoteSigner, "--source-ref"} {
		if !strings.Contains(script, want) {
			t.Fatalf("the check does not name %q: %s", want, script)
		}
	}
	if verify >= check || verify >= latest {
		t.Fatal("a copy comes before the check of the attestation")
	}
	if index(stepVersion) >= index(stepLatest) {
		t.Fatal("latest moves before the version tag: the version tag comes first")
	}
}
