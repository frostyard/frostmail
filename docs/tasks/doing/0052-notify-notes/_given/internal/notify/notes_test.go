package notify

// CONTRACT TEST for task card T-0052 (docs/tasks). Do not edit.

import (
	"reflect"
	"strings"
	"testing"
)

func TestNotesOnePerMessage(t *testing.T) {
	got := Notes([]Mail{
		{ID: 1, FromName: " Ann Example ", FromAddr: "ann@x.test", Subject: "Lunch", Preview: "  Are you\n free \t at noon? "},
		{ID: 2, FromAddr: "bob@x.test", Subject: "  ", Preview: ""},
		{ID: 3, Subject: "Report", Preview: "See attached."},
	})
	want := []Note{
		{Summary: "Ann Example", Body: "Lunch\nAre you free at noon?", MessageID: 1},
		{Summary: "bob@x.test", Body: "(no subject)", MessageID: 2},
		{Summary: "Unknown Sender", Body: "Report\nSee attached.", MessageID: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Notes = %#v", got)
	}
}

func TestNotesTruncatesPreviews(t *testing.T) {
	long := strings.Repeat("é", 130)
	got := Notes([]Mail{{ID: 7, FromName: "Ann", Subject: "S", Preview: long}})
	if len(got) != 1 {
		t.Fatalf("got %d notes", len(got))
	}
	want := "S\n" + strings.Repeat("é", 120) + "…"
	if got[0].Body != want {
		t.Errorf("Body = %q", got[0].Body)
	}
	exact := strings.Repeat("a", 120)
	if b := Notes([]Mail{{ID: 8, FromName: "Ann", Subject: "S", Preview: exact}})[0].Body; b != "S\n"+exact {
		t.Errorf("120 runes were cut: %q", b)
	}
}

func TestNotesGroupFourOrMore(t *testing.T) {
	mail := []Mail{
		{ID: 1, FromName: "Ann", Subject: "a"},
		{ID: 2, FromAddr: "bob@x.test", Subject: "b"},
		{ID: 3, FromName: "Ann", Subject: "c"},
		{ID: 4, FromName: "Carol", Subject: "d"},
	}
	got := Notes(mail)
	want := []Note{{Summary: "4 new messages", Body: "From Ann, bob@x.test and Carol"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Notes = %#v", got)
	}
	mail = append(mail, Mail{ID: 5, FromName: "Dan"}, Mail{ID: 6, FromName: "Eve"}, Mail{ID: 7, FromName: "Dan"})
	got = Notes(mail)
	want = []Note{{Summary: "7 new messages", Body: "From Ann, bob@x.test, Carol and 2 others"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Notes = %#v", got)
	}
	got = Notes([]Mail{{ID: 1, FromName: "Ann"}, {ID: 2, FromName: "Ann"}, {ID: 3, FromName: "Ann"}, {ID: 4, FromName: "Ann"}})
	want = []Note{{Summary: "4 new messages", Body: "From Ann"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("one sender = %#v", got)
	}
	got = Notes([]Mail{{ID: 1, FromName: "Ann"}, {ID: 2, FromName: "Bob"}, {ID: 3, FromName: "Ann"}, {ID: 4, FromName: "Bob"}})
	want = []Note{{Summary: "4 new messages", Body: "From Ann and Bob"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("two senders = %#v", got)
	}
}

func TestNotesNone(t *testing.T) {
	if got := Notes(nil); got != nil {
		t.Errorf("Notes(nil) = %#v", got)
	}
}
