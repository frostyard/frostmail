package main

// CONTRACT TEST for task card T-0138 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestPeopleEditCommands(t *testing.T) {
	srv, ids := pimFixture(t)
	run := func(args ...string) string { return mustRun(t, append([]string{"--socket", srv.Socket}, args...)...) }
	fails := func(args ...string) {
		t.Helper()
		if out, err := runMailctl(t, append([]string{"--socket", srv.Socket}, args...)...); err == nil {
			t.Errorf("mailctl %q succeeded: %s", args, out)
		}
	}

	// people show names each contact, for edit and rm.
	m := regexp.MustCompile(`(?m)^Contact (\d+)$`).FindStringSubmatch(run("people", "show", itoa64(ids.ada)))
	if m == nil {
		t.Fatal("people show has no Contact line")
	}
	ada := m[1]

	if got := run("people", "edit", ada, "--title", "Countess of Lovelace",
		"--email", "work=ada@engines.example", "--email", "ada@example.com"); got != "updated Ada Lovelace\n" {
		t.Errorf("people edit = %q", got)
	}
	show := run("people", "show", itoa64(ids.ada))
	if !strings.Contains(show, "Email: ada@engines.example (work)\nEmail: ada@example.com\n") {
		t.Errorf("people show after the edit =\n%s", show)
	}
	var person api.Person
	if err := json.Unmarshal([]byte(run("--json", "people", "show", itoa64(ids.ada))), &person); err != nil ||
		person.Contacts[0].Title != "Countess of Lovelace" || person.Contacts[0].Note != "Countess" {
		t.Errorf("the edited contact = %+v, %v", person, err)
	}

	added := run("people", "add", "--first", "Kai", "--last", "Berg", "--email", "home=kai@berg.test", "--phone", "+1 555 0100")
	m = regexp.MustCompile(`^added Kai Berg \(contact (\d+)\)\n$`).FindStringSubmatch(added)
	if m == nil {
		t.Fatalf("people add = %q", added)
	}
	if got := cells(run("people", "ls", "kai")); len(got) != 2 || got[1][1] != "Kai Berg" || got[1][2] != "kai@berg.test" {
		t.Errorf("people ls kai = %q", got)
	}
	if got := run("people", "rm", m[1]); got != "deleted contact "+m[1]+"\n" {
		t.Errorf("people rm = %q", got)
	}
	if got := run("people", "ls", "kai"); got != "no people\n" {
		t.Errorf("after people rm, people ls kai = %q", got)
	}

	var w api.ContactWrite
	if err := json.Unmarshal([]byte(run("--json", "people", "add", "--company", "Lumen")), &w); err != nil ||
		w.RestoreID == "" || w.Person == nil || w.Person.DisplayName != "Lumen" {
		t.Errorf("--json people add = %+v, %v", w, err)
	}

	fails("people", "edit", ada)
	fails("people", "edit", "99999", "--title", "x")
	fails("people", "add", "--email", "not an address")
	fails("people", "rm", "x")
}
