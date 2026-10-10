// CONTRACT TEST for task card T-0097 (docs/tasks). Do not edit.
package engine_test

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
)

// TestInvitationOwnCopyIsNoConflict: inviting one of your accounts from
// another puts the organizer's copy of the event in your calendars too;
// the same event (by UID) is no conflict with itself.
func TestInvitationOwnCopyIsNoConflict(t *testing.T) {
	e := newInviteEnv(t)
	ctx := t.Context()
	before := summaries(e.invitation(t, e.mail(t, request("REQUEST", 1, userLine))).Conflicts)
	if len(before) == 0 {
		t.Fatal("the fixture has no conflicts to keep")
	}
	if err := e.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "imap.example", Port: 993, TLS: api.TLSModeTLS, Username: "maria"}
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "maria@example.com",
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		if err := tx.SetService(ctx, acct.ID, api.ServiceKindCalendar, true, "https://dav.example/"); err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar, []store.RemoteCollection{{Href: "/m/", Name: "Maria"}})
		if err != nil {
			return err
		}
		from, to := pimsync.Window(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
		_, err = pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: cols[0].ID, Href: "/m/launch.ics",
			Raw: event("launch@example.com", "20261015T140000Z", "20261015T150000Z", "Launch review",
				"ORGANIZER;CN=Maria Lopez:mailto:maria@example.com")}, calendar.Options{Local: time.UTC}, from, to)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got := summaries(e.invitation(t, e.mail(t, request("REQUEST", 1, userLine))).Conflicts)
	if !slices.Equal(got, before) {
		t.Errorf("conflicts = %q, want %q", got, before)
	}
}
