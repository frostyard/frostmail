package notify

import (
	"slices"
	"testing"
)

// P-805: Reply always, Archive and Trash only with their mailboxes.
func TestButtons(t *testing.T) {
	for _, c := range []struct {
		mail Mail
		want []string
	}{
		{Mail{}, []string{ActionReply}},
		{Mail{Archive: true}, []string{ActionReply, ActionArchive}},
		{Mail{Trash: true}, []string{ActionReply, ActionTrash}},
		{Mail{Archive: true, Trash: true}, []string{ActionReply, ActionArchive, ActionTrash}},
	} {
		if got := Buttons(c.mail); !slices.Equal(got, c.want) {
			t.Errorf("Buttons(%+v) = %q, want %q", c.mail, got, c.want)
		}
	}
}
