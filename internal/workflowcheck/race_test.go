package workflowcheck

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// CI runs the race detector as shards, each on a runner of its own (make
// race, tools/testshard), and main's rulesets require the check named
// "Race detector". That check must fail unless every shard ran and passed:
// the matrix runs shards 1 to N of the same N, no shard's failure is
// ignored, and the job with the required name runs whatever the shards did
// (a job that needs a failed job is skipped unless its condition is
// always(), and a skipped required check counts as passed) and passes only
// on their success.
func TestRaceDetectorCheckNeedsEveryShard(t *testing.T) {
	root, ok := workflows(t)["ci.yml"]
	if !ok {
		t.Fatal("no ci.yml")
	}
	jobs := get(root, "jobs")
	shard, gate := get(jobs, "race-shard"), get(jobs, "race")
	if shard == nil || gate == nil {
		t.Fatal("ci.yml: want the jobs race-shard (the shards) and race (the required check)")
	}

	matrix := get(get(shard, "strategy"), "matrix")
	list := get(matrix, "shard")
	if list == nil || list.Kind != yaml.SequenceNode || len(list.Content) < 2 {
		t.Fatal("ci.yml: race-shard has no matrix of 2 or more shards")
	}
	if len(matrix.Content) != 2 {
		t.Error("ci.yml: the matrix of race-shard may hold only the shards: an exclude would drop one")
	}
	n := len(list.Content)
	for i, v := range list.Content {
		if v.Value != strconv.Itoa(i+1) {
			t.Errorf("ci.yml:%d: shard %d of the matrix is %q; the shards are 1 to %d in order", v.Line, i+1, v.Value, n)
		}
	}
	if get(shard, "continue-on-error") != nil {
		t.Error("ci.yml: race-shard sets continue-on-error, so a failed shard would count as passed")
	}
	steps := get(shard, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		t.Fatal("ci.yml: race-shard has no steps")
	}
	want, runs := fmt.Sprintf("${{ matrix.shard }}/%d", n), 0
	for _, step := range steps.Content {
		run := get(step, "run")
		if run == nil || !strings.Contains(run.Value, "make race") {
			continue
		}
		runs++
		if s := get(get(step, "env"), "RACE_SHARD"); s == nil || s.Value != want {
			t.Errorf("ci.yml:%d: the step that runs make race must set RACE_SHARD to %q, for the matrix has %d shards", run.Line, want, n)
		}
		// The whole line: nothing after it may hide its failure, and
		// -count=1 keeps every test result out of a cache.
		const command = "make race RACE_FLAGS=-count=1"
		if !slices.ContainsFunc(strings.Split(run.Value, "\n"), func(l string) bool { return strings.TrimSpace(l) == command }) {
			t.Errorf("ci.yml:%d: the step must run %q on a line of its own", run.Line, command)
		}
		if get(step, "continue-on-error") != nil || get(step, "if") != nil || get(step, "shell") != nil {
			t.Errorf("ci.yml:%d: the step that runs make race must always run, and its failure must fail the shard", run.Line)
		}
	}
	if runs != 1 {
		t.Errorf("ci.yml: race-shard has %d steps that run make race, want 1", runs)
	}

	if name := get(gate, "name"); name == nil || name.Value != "Race detector" {
		t.Error(`ci.yml: the job race must keep the name "Race detector", which main's rulesets require`)
	}
	if cond := get(gate, "if"); cond == nil || cond.Value != "always()" {
		t.Error("ci.yml: the job race must have if: always(), or a failed shard leaves the required check skipped")
	}
	if get(gate, "continue-on-error") != nil {
		t.Error("ci.yml: the job race sets continue-on-error")
	}
	needs := get(gate, "needs")
	if needs == nil || (needs.Value != "race-shard" &&
		!slices.ContainsFunc(needs.Content, func(v *yaml.Node) bool { return v.Value == "race-shard" })) {
		t.Error("ci.yml: the job race must need race-shard")
	}
	checks := 0
	if steps = get(gate, "steps"); steps != nil {
		for _, step := range steps.Content {
			if passesOnlyOnSuccess(step, "${{ needs.race-shard.result }}") {
				checks++
			}
		}
	}
	if checks == 0 {
		t.Error(`ci.yml: the job race needs a step that fails unless needs.race-shard.result is "success"`)
	}
}

// passesOnlyOnSuccess reports whether a step puts the expression result in
// a variable of its environment and runs `test "$NAME" = success` on a
// line of its own, with nothing that lets the step pass or skip otherwise.
func passesOnlyOnSuccess(step *yaml.Node, result string) bool {
	env, run := get(step, "env"), get(step, "run")
	if env == nil || env.Kind != yaml.MappingNode || run == nil {
		return false
	}
	if get(step, "continue-on-error") != nil || get(step, "if") != nil || get(step, "shell") != nil {
		return false
	}
	for i := 0; i+1 < len(env.Content); i += 2 {
		if env.Content[i+1].Value != result {
			continue
		}
		check := fmt.Sprintf(`test "$%s" = success`, env.Content[i].Value)
		for line := range strings.SplitSeq(run.Value, "\n") {
			if strings.TrimSpace(line) == check {
				return true
			}
		}
	}
	return false
}

func TestPassesOnlyOnSuccess(t *testing.T) {
	const result = "${{ needs.race-shard.result }}"
	for doc, want := range map[string]bool{
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$SHARDS\" = success":                                  true,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: |\n  echo \"shards: $SHARDS\"\n  test \"$SHARDS\" = success": true,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$SHARDS\" != failure":                                 false,
		"env: {SHARDS: '${{ needs.verify.result }}'}\nrun: test \"$SHARDS\" = success":                                      false,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$OTHER\" = success":                                   false,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$SHARDS\" = success || true":                          false,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$SHARDS\" = success\ncontinue-on-error: true":         false,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$SHARDS\" = success\nif: github.ref == 'x'":           false,
		"env: {SHARDS: '${{ needs.race-shard.result }}'}\nrun: test \"$SHARDS\" = success\nshell: bash {0} || true":         false,
		"run: test \"$SHARDS\" = success": false,
	} {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(doc), &node); err != nil || len(node.Content) == 0 {
			t.Fatalf("parse %q: %v", doc, err)
		}
		if got := passesOnlyOnSuccess(node.Content[0], result); got != want {
			t.Errorf("passesOnlyOnSuccess(%q) = %v, want %v", doc, got, want)
		}
	}
}
