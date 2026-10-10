package engine_test

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// asKind makes the env's account one of kind's, whose provider profile
// decides who tells the organizer (ADR-0022).
func (e *inviteEnv) asKind(t *testing.T, kind api.AccountKind) {
	t.Helper()
	if err := e.srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE accounts SET kind = ? WHERE id = ?`, kind, e.acct)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

const agentOrganizer = "ORGANIZER;CN=Maria Lopez;SCHEDULE-AGENT=CLIENT:mailto:maria@example.com"

// TestRespondMailsWhatICloudWillNot: iCloud schedules only for copies its
// own scheduling delivered. An emailed invitation accepted here is
// answered by mail, and its new copy leaves scheduling to the client, so
// changing the answer later mails again.
func TestRespondMailsWhatICloudWillNot(t *testing.T) {
	e := newInviteEnv(t)
	e.asKind(t, api.AccountKindICloud)
	id := e.mail(t, request("REQUEST", 1, userLine))
	ev := e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatAccepted})
	items, cals := e.outbox(t)
	if len(items) != 1 || len(cals) != 1 || strings.Contains(cals[0], "SCHEDULE-") ||
		!strings.Contains(cals[0], "PARTSTAT=ACCEPTED") {
		t.Fatalf("outbox = %+v, %q", items, cals)
	}
	e.pass(t)
	copyOf := e.serverCopy(t, "launch@example.com")
	if !strings.Contains(copyOf, agentOrganizer) || strings.Count(copyOf, "SCHEDULE-AGENT") != 1 ||
		!strings.Contains(copyOf, "PARTSTAT=ACCEPTED") {
		t.Errorf("the server's copy:\n%s", copyOf)
	}
	e.respond(t, &api.CalendarRespondParams{EventID: &ev.ID, Answer: api.PartStatTentative})
	items, cals = e.outbox(t)
	if len(items) != 2 || strings.Contains(cals[1], "SCHEDULE-") || !strings.Contains(cals[1], "PARTSTAT=TENTATIVE") {
		t.Errorf("after changing the answer: %+v, %q", items, cals)
	}
	e.pass(t)
	if copyOf := e.serverCopy(t, "launch@example.com"); strings.Count(copyOf, "SCHEDULE-AGENT=CLIENT") != 1 {
		t.Errorf("the server's copy:\n%s", copyOf)
	}
}

// TestRespondLeavesGoogleItsCopies: Google tells the organizer about a
// copy a client creates; nothing is mailed and the copy is the server's to
// schedule.
func TestRespondLeavesGoogleItsCopies(t *testing.T) {
	e := newInviteEnv(t)
	e.asKind(t, api.AccountKindGmail)
	id := e.mail(t, request("REQUEST", 1, userLine))
	e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatAccepted})
	if items, _ := e.outbox(t); len(items) != 0 {
		t.Errorf("mailed a reply Google sends: %+v", items)
	}
	e.pass(t)
	if copyOf := e.serverCopy(t, "launch@example.com"); strings.Contains(copyOf, "SCHEDULE-AGENT") {
		t.Errorf("the server's copy:\n%s", copyOf)
	}
}

// TestRespondThroughICloudsOwnCopy: a copy iCloud's scheduling put in the
// calendar is answered by its put alone.
func TestRespondThroughICloudsOwnCopy(t *testing.T) {
	e := newInviteEnv(t)
	e.asKind(t, api.AccountKindICloud)
	e.dav.Put(e.work+"launch.ics", []byte(strings.Replace(request("REQUEST", 1, userLine), "METHOD:REQUEST\r\n", "", 1)))
	e.pass(t)
	id := e.mail(t, request("REQUEST", 1, userLine))
	e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatAccepted})
	if items, _ := e.outbox(t); len(items) != 0 {
		t.Errorf("mailed a reply iCloud sends: %+v", items)
	}
	e.pass(t)
	if copyOf := e.serverCopy(t, "launch@example.com"); strings.Contains(copyOf, "SCHEDULE-AGENT") ||
		!strings.Contains(copyOf, "PARTSTAT=ACCEPTED") {
		t.Errorf("the server's copy:\n%s", copyOf)
	}
}
