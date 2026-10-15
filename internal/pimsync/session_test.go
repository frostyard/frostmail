package pimsync_test

import (
	"context"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davsession"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/store"
)

// The Google write session runs to the end against davtest, refusing a
// SEQUENCE that goes back as Google does, and leaves nothing behind: the
// script works before it is recorded against Google.
func TestGoogleSessionOnDavtest(t *testing.T) {
	e := newEnv(t, davtest.Options{KeepSequence: true}, api.ServiceKindCalendar, api.ServiceKindContacts)
	ctx := t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE accounts SET kind = 'gmail' WHERE id = ?`, e.acct.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	work := e.dav.Calendar("work", "Work", "#3366cc", "VEVENT")
	book := e.dav.AddressBook("contacts", "Contacts")
	eng := engine.New(engine.Deps{DB: e.db, PIM: e.m})
	var steps []string
	err := davsession.Google(ctx, davsession.Session{
		Calendar: eng.Calendar(),
		People:   eng.People(),
		Pass:     func(ctx context.Context) error { return e.m.Pass(ctx, e.acct.ID) },
		Step:     func(name string) { steps = append(steps, name) },
	})
	if err != nil {
		t.Fatalf("after %q: %v", steps, err)
	}
	if len(steps) != 23 {
		t.Errorf("steps = %q", steps)
	}
	if left := append(e.dav.Paths(work), e.dav.Paths(book)...); len(left) != 0 {
		t.Errorf("the session left %q on the server", left)
	}
	rows, err := e.db.Events(ctx, store.EventsFilter{})
	if err != nil || len(rows) != 0 {
		t.Errorf("events left here: %+v, %v", rows, err)
	}
}
