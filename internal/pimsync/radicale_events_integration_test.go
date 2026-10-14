//go:build integration

package pimsync_test

// Event writes against Radicale in the frostmail-mailtest container (make
// engine-it; plan 0009, Phase 3), as test2@mailtest.test, whose calendar
// no other test uses. The manager is not started, so every pass is the
// test's own. A new event, an edit applied again after another client's
// change, each scope of a series' edit and delete, and an undo reach the
// server; verify finds both sides the same.

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func TestRadicaleEventWrites(t *testing.T) {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	const user, password = "test2@mailtest.test", "frostmail-test"
	ctx := t.Context()
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the test container's certificate
	}}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	start := "https://" + host + ":5232/"
	var acct store.Account
	err = db.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: user}
		if acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: user,
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server}); err != nil {
			return err
		}
		return tx.SetService(ctx, acct.ID, api.ServiceKindCalendar, true, start)
	})
	if err != nil {
		t.Fatal(err)
	}
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	if err := sec.Set(ctx, secrets.AccountPassword(acct.ID), password); err != nil {
		t.Fatal(err)
	}
	m := pimsync.New(db, sec, slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{HTTP: insecure, Local: time.UTC})
	cal := engine.New(engine.Deps{DB: db, PIM: m}).Calendar()
	pass := func() {
		t.Helper()
		if err := m.Pass(ctx, acct.ID); err != nil {
			t.Fatalf("Pass: %v", err)
		}
	}
	pass() // discovery makes Radicale create the user's calendar

	opts := davx.Options{HTTP: insecure, Authorization: func(context.Context) (string, error) {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
	}}
	home, err := davx.Discover(ctx, start, davx.Calendars, opts)
	if err != nil {
		t.Fatal(err)
	}
	other, err := davx.New(home, opts)
	if err != nil {
		t.Fatal(err)
	}
	calendar := "/" + strings.ReplaceAll(user, "@", "%40") + "/calendar/"
	// The container's reset keeps Radicale's data: start from an empty
	// calendar.
	list, err := other.List(ctx, calendar)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range list {
		if err := other.Delete(ctx, o.Href, ""); err != nil {
			t.Fatal(err)
		}
	}
	href := calendar + "radicale-sync.ics"
	series := func(description string) []byte {
		return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Frostmail//IT//EN\r\nBEGIN:VEVENT\r\n" +
			"UID:radicale-sync\r\nDTSTAMP:20261001T000000Z\r\nDTSTART;TZID=Europe/Berlin:20261015T090000\r\n" +
			"DTEND;TZID=Europe/Berlin:20261015T093000\r\nRRULE:FREQ=WEEKLY;COUNT=6\r\nSUMMARY:Sync\r\n" +
			"DESCRIPTION:" + description + "\r\nX-CUSTOM:keep me\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	}
	server := func(h string) string {
		t.Helper()
		obj, err := other.Get(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		return string(obj.Data)
	}
	if _, err := other.Put(ctx, href, series("Notes in the doc"), davx.Calendars, ""); err != nil {
		t.Fatal(err)
	}
	pass()
	rows, err := db.Events(ctx, store.EventsFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("events = %+v, %v", rows, err)
	}
	id := rows[0].ID
	occurrence := func(key string, s api.EventScope) (*string, *api.EventScope) { return &key, &s }

	// A new event.
	at := time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC)
	made, err := cal.Create(ctx, &api.CalendarCreateParams{Fields: api.EventFields{Summary: ptr2("Review"),
		AllDay: ptr2(false), Start: &at, End: ptr2(at.Add(time.Hour)), TimeZone: ptr2("America/New_York")}})
	if err != nil {
		t.Fatal(err)
	}
	pass()
	if got := server(calendar + made.Event.UID + ".ics"); !strings.Contains(got, "DTSTART;TZID=America/New_York:20261016T090000") ||
		!strings.Contains(got, "BEGIN:VTIMEZONE") {
		t.Errorf("the new event on Radicale is %s", got)
	}

	// An edit of one occurrence while another client changes the notes:
	// both survive.
	rid, scope := occurrence("2026-10-22T07:00:00.000Z", api.EventScopeThis)
	if _, err := cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: rid, Scope: scope,
		Fields: api.EventFields{Location: ptr2("Room 2")}}); err != nil {
		t.Fatal(err)
	}
	obj, err := other.Get(ctx, href)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Put(ctx, href, series("Notes moved to the wiki"), davx.Calendars, obj.ETag); err != nil {
		t.Fatal(err)
	}
	pass()
	got := server(href)
	for _, line := range []string{"RECURRENCE-ID;TZID=Europe/Berlin:20261022T090000", "LOCATION:Room 2",
		"DESCRIPTION:Notes moved to the wiki", "X-CUSTOM:keep me"} {
		if !strings.Contains(got, line+"\r\n") {
			t.Errorf("after the edit Radicale lacks %q:\n%s", line, got)
		}
	}

	// All Events moves the series an hour later; All Future Events splits
	// it; This Event deletes one occurrence.
	rid, scope = occurrence("2026-10-29T08:00:00.000Z", api.EventScopeAll)
	nine := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC) // 10:00 in Berlin, after DST ends
	if _, err := cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: rid, Scope: scope,
		Fields: api.EventFields{AllDay: ptr2(false), Start: &nine, End: ptr2(nine.Add(30 * time.Minute)), TimeZone: ptr2("Europe/Berlin")}}); err != nil {
		t.Fatal(err)
	}
	rid, scope = occurrence("2026-11-05T09:00:00.000Z", api.EventScopeFuture)
	split, err := cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: rid, Scope: scope,
		Fields: api.EventFields{Summary: ptr2("Sync v2")}})
	if err != nil {
		t.Fatal(err)
	}
	rid, scope = occurrence("2026-10-15T08:00:00.000Z", api.EventScopeThis)
	gone, err := cal.Delete(ctx, &api.CalendarDeleteParams{ID: id, RecurrenceID: rid, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	pass()
	got = server(href)
	for _, line := range []string{"DTSTART;TZID=Europe/Berlin:20261015T100000", "RECURRENCE-ID;TZID=Europe/Berlin:20261022T100000",
		"RRULE:FREQ=WEEKLY;UNTIL=20261105T085959Z", "EXDATE;TZID=Europe/Berlin:20261015T100000"} {
		if !strings.Contains(got, line+"\r\n") {
			t.Errorf("after the scoped writes Radicale lacks %q:\n%s", line, got)
		}
	}
	if next := server(calendar + split.Event.UID + ".ics"); !strings.Contains(next, "SUMMARY:Sync v2") ||
		!strings.Contains(next, "DTSTART;TZID=Europe/Berlin:20261105T100000") || !strings.Contains(next, "COUNT=3") {
		t.Errorf("the new series on Radicale is %s", next)
	}

	// The delete's undo.
	if err := cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: gone.RestoreID}); err != nil {
		t.Fatal(err)
	}
	pass()
	if strings.Contains(server(href), "EXDATE") {
		t.Error("the delete's undo did not reach Radicale")
	}
	pass()
	checks, err := m.Verify(ctx, acct.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Server != 3 || checks[0].Local != 3 {
		t.Errorf("verify = %+v, %v", checks, err)
	}
}
