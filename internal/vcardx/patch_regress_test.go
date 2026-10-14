package vcardx

import (
	"strings"
	"testing"
)

// A list that grows after its last line, where the next line is a single
// field the same patch rewrites: Frostmail's own new card, as the M5.5
// Phase 2 check on Gmail first edited it.
func TestPatchGrowsAListBeforeARewrittenLine(t *testing.T) {
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "UID:c1", "FN:Frostmail Check", "N:Check;Frostmail;;;",
		"EMAIL;TYPE=INTERNET;TYPE=HOME:check@frostmail.test", "NOTE:Made by the check", "END:VCARD")
	got := patched(t, src, Fields{
		Emails: &[]Labeled{{Label: "home", Value: "check@frostmail.test"}, {Label: "work", Value: "lab@frostmail.test"}},
		Note:   ptr("Edited"),
	})
	want := strings.Replace(src, "NOTE:Made by the check\r\n",
		"EMAIL;TYPE=INTERNET;TYPE=WORK:lab@frostmail.test\r\nNOTE:Edited\r\n", 1)
	expect(t, got, want)
}
