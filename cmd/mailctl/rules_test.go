package main

// CONTRACT TEST for task card T-0114 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

const twoRules = `[
  {"name": "Shop", "enabled": true,
   "conditions": {"match": "all", "conditions": [{"field": "from", "op": "contains", "value": "shop"}]},
   "actions": [{"kind": "read"}, {"kind": "stop"}]},
  {"name": "Carol", "enabled": false,
   "conditions": {"match": "any", "conditions": [{"field": "from", "op": "is", "value": "carol@mailtest.test"}]},
   "actions": [{"kind": "flag", "color": 3}]}
]`

func rulesJSON(t *testing.T, socket string) []api.Rule {
	t.Helper()
	out, err := runMailctl(t, "--socket", socket, "--json", "rules", "ls")
	if err != nil {
		t.Fatal(err)
	}
	var rules []api.Rule
	if err := json.Unmarshal([]byte(out), &rules); err != nil {
		t.Fatalf("rules ls --json = %q: %v", out, err)
	}
	return rules
}

func ruleNames(rules []api.Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.Name
		if !r.Enabled {
			out[i] += " (off)"
		}
	}
	return out
}

func writeFile(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRulesCommands(t *testing.T) {
	srv := rpctest.Start(t)
	run := func(args ...string) (string, error) {
		return runMailctl(t, append([]string{"--socket", srv.Socket, "rules"}, args...)...)
	}
	if out, err := run("ls"); err != nil || out != "no rules\n" {
		t.Fatalf("ls with none = %q, %v", out, err)
	}
	if out, err := run("import", writeFile(t, "rules.json", twoRules)); err != nil || out != "added 2 rules\n" {
		t.Fatalf("import = %q, %v", out, err)
	}
	out, err := run("ls")
	want := "#  ID  ON  NAME   PROBLEM\n" +
		"1  1   x   Shop   -\n" +
		"2  2   -   Carol  -\n"
	if err != nil || out != want {
		t.Fatalf("ls = %q, %v; want %q", out, err, want)
	}
	rules := rulesJSON(t, srv.Socket)
	if !slices.Equal(ruleNames(rules), []string{"Shop", "Carol (off)"}) || len(rules[0].Actions) != 2 {
		t.Fatalf("rules = %+v", rules)
	}

	if out, err := run("on", "2"); err != nil || out != "updated 1 rule\n" {
		t.Fatalf("on = %q, %v", out, err)
	}
	if out, err := run("off", "1", "2"); err != nil || out != "updated 2 rules\n" {
		t.Fatalf("off = %q, %v", out, err)
	}
	if got := ruleNames(rulesJSON(t, srv.Socket)); !slices.Equal(got, []string{"Shop (off)", "Carol (off)"}) {
		t.Fatalf("after on and off: %v", got)
	}
	if out, err := run("mv", "2", "1"); err != nil || out != "moved rule 2 to 1\n" {
		t.Fatalf("mv = %q, %v", out, err)
	}
	if got := ruleNames(rulesJSON(t, srv.Socket)); !slices.Equal(got, []string{"Carol (off)", "Shop (off)"}) {
		t.Fatalf("after mv: %v", got)
	}

	// --json rules ls is what import reads.
	exported, err := runMailctl(t, "--socket", srv.Socket, "--json", "rules", "ls")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := run("import", writeFile(t, "exported.json", exported)); err != nil || out != "added 2 rules\n" {
		t.Fatalf("import of an export = %q, %v", out, err)
	}
	if got := ruleNames(rulesJSON(t, srv.Socket)); !slices.Equal(got, []string{"Carol (off)", "Shop (off)", "Carol (off)", "Shop (off)"}) {
		t.Fatalf("after the round trip: %v", got)
	}
	if out, err := run("rm", "3", "4"); err != nil || out != "removed 2 rules\n" {
		t.Fatalf("rm = %q, %v", out, err)
	}

	bad := writeFile(t, "bad.json", `[{"name": "Bad", "enabled": true, "conditions": {"match": "all", "conditions": []},
		"actions": [{"kind": "flag", "color": 9}]}]`)
	for _, args := range [][]string{
		{"on"},
		{"off", "x"},
		{"mv", "2"},
		{"mv", "2", "0"},
		{"mv", "x", "1"},
		{"rm", "99"},
		{"apply"},
		{"import", writeFile(t, "empty.json", "[]")},
		{"import", writeFile(t, "junk.json", "not json")},
		{"import", bad},
		{"import", filepath.Join(t.TempDir(), "missing.json")},
	} {
		if _, err := run(args...); err == nil {
			t.Errorf("mailctl rules %s succeeded", strings.Join(args, " "))
		}
	}
	if got := len(rulesJSON(t, srv.Socket)); got != 2 {
		t.Errorf("%d rules after the refusals, want 2", got)
	}
}

func TestRulesApplyCommand(t *testing.T) {
	e := opsFixture(t)
	ctx := t.Context()
	if _, err := e.c.Rule().Create(ctx, &api.RuleCreateParams{
		Name: "Q3",
		Conditions: api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
			{Field: api.ConditionFieldSubject, Op: api.ConditionOpContains, Value: "Q3"}}},
		Actions: []api.RuleAction{{Kind: api.RuleActionKindMove, MailboxID: &e.archive}},
	}); err != nil {
		t.Fatal(err)
	}
	args := []string{"--socket", e.srv.Socket, "rules", "apply"}
	for _, id := range e.ids {
		args = append(args, idArg(id))
	}
	out, err := runMailctl(t, args...)
	if err != nil || out != "1 of 5 messages matched a rule\n" {
		t.Fatalf("apply = %q, %v", out, err)
	}
	e.waitServerCount(t, "Archive", 1)

	other := slices.IndexFunc(e.ids, func(id int64) bool { return e.get(t, id).Summary.Subject != "Q3 numbers" })
	out, err = runMailctl(t, "--socket", e.srv.Socket, "--json", "rules", "apply", idArg(e.ids[other]))
	var got api.RuleApplied
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || got.Matched != 0 {
		t.Fatalf("apply --json = %q, %v", out, err)
	}
}
