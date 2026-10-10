package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// TestAcceptIntoTheServersDefault: iCloud listed a shared calendar first,
// so it became the default, and an accepted invitation went there. The
// server names the user's default calendar (RFC 6638); without a choice
// of the user's, accepting puts the invitation there, and a choice of the
// user's wins.
func TestAcceptIntoTheServersDefault(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	dav := davtest.New(t, davtest.Options{ScheduleDefault: davtest.CalendarsHome + "calendar/"})
	srv := rpctest.StartWith(t, rpctest.Options{
		PIM: &pimsync.Config{HTTP: dav.Client, Interval: time.Hour, Local: time.UTC},
		Now: func() time.Time { return now },
	})
	e := &inviteEnv{srv: srv, c: srv.Dial(t), dav: dav}
	e.acct = davAccount(t, e.c)
	maint := dav.Calendar("a-maintenance", "Maintenance", "")
	cal := dav.Calendar("calendar", "Calendar", "")
	e.work = cal
	if _, err := e.c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: e.acct, Service: api.ServiceKindCalendar,
		Enabled: true, URL: ptr(dav.URL)}); err != nil {
		t.Fatal(err)
	}
	e.pass(t)
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, e.acct, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		e.inbox = mbs[0].ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	collection := func(href string) store.Collection {
		t.Helper()
		cols, _ := srv.DB.Collections(ctx, store.CollectionFilter{AccountID: e.acct, Kind: api.CollectionKindCalendar})
		for _, c := range cols {
			if c.Href == href {
				return c
			}
		}
		t.Fatalf("no calendar %s in %+v", href, cols)
		return store.Collection{}
	}
	if !collection(maint).IsDefault {
		t.Fatal("the first listed calendar is not the default before the server is asked")
	}

	id := e.mail(t, request("REQUEST", 1, userLine))
	ev := e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatAccepted})
	if ev.CalendarID != collection(cal).ID || !collection(cal).IsDefault || collection(maint).IsDefault {
		t.Errorf("accepted into calendar %d; defaults: maintenance %v, calendar %v", ev.CalendarID,
			collection(maint).IsDefault, collection(cal).IsDefault)
	}

	// The user makes Maintenance the default; the server's no longer wins.
	if _, err := e.c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: collection(maint).ID, IsDefault: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	second := e.mail(t, strings.ReplaceAll(request("REQUEST", 1, userLine), "launch@example.com", "second@example.com"))
	ev = e.respond(t, &api.CalendarRespondParams{MessageID: &second, Answer: api.PartStatAccepted})
	if ev.CalendarID != collection(maint).ID || !collection(maint).IsDefault {
		t.Errorf("after the user's choice: accepted into calendar %d", ev.CalendarID)
	}
}
