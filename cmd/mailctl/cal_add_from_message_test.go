package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

func TestCalAddFromMessage(t *testing.T) {
	srv, ids := pimFixture(t)
	ctx := t.Context()
	raw := "From: Airline <noreply@airline.example>\r\nTo: user@example.com\r\nSubject: Your flight\r\n" +
		"Date: Fri, 09 Oct 2026 14:00:00 +0000\r\nMessage-ID: <f@airline.example>\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: text/calendar; charset=UTF-8; method=PUBLISH\r\n\r\n" +
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nMETHOD:PUBLISH\r\nBEGIN:VEVENT\r\nUID:flight@airline.example\r\n" +
		"DTSTAMP:20261009T140000Z\r\nDTSTART:20261020T083000Z\r\nDTEND:20261020T104500Z\r\nSUMMARY:Flight LH 400\r\n" +
		"ORGANIZER:mailto:noreply@airline.example\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	blobID, err := srv.Blobs.Put(ctx, bytes.NewReader([]byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var msg int64
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, ids.acct, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		got, err := tx.InsertHeaders(ctx, ids.acct, mbs[0].ID, []store.MessageHeader{{UID: 1, Subject: "Your flight",
			InternalDate: time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), From: store.Address{Addr: "noreply@airline.example"}}})
		if err != nil {
			return err
		}
		msg = got[0]
		return tx.SetBody(ctx, msg, blobID)
	}); err != nil {
		t.Fatal(err)
	}
	id := itoa64(msg)

	for _, bad := range [][]string{{}, {"soon"}, {"0"}, {"99999"}, {id, "--calendar", itoa64(ids.home)}, {id, "--calendar", "99999"}} {
		if out, err := runMailctl(t, append([]string{"--socket", srv.Socket, "cal", "add-from-message"}, bad...)...); err == nil {
			t.Errorf("add-from-message %q succeeded: %s", bad, out)
		}
	}

	out := mustRun(t, "--socket", srv.Socket, "cal", "add-from-message", id)
	m := regexp.MustCompile(`^added event (\d+) to calendar ` + itoa64(ids.work) + `: Flight LH 400\n$`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("add-from-message = %q", out)
	}
	agenda := mustRun(t, "--socket", srv.Socket, "cal", "agenda", "--from", "2026-10-20", "--days", "1", "--zone", "UTC", "--ids")
	if want := "2026-10-20 Tue\n  08:30–10:45  Flight LH 400  [" + m[1] + "]\n"; agenda != want {
		t.Errorf("agenda =\n%s\nwant\n%s", agenda, want)
	}
	js := mustRun(t, "--socket", srv.Socket, "--json", "cal", "agenda", "--from", "2026-10-20", "--days", "1", "--zone", "UTC")
	var rows []api.Occurrence
	if err := json.Unmarshal([]byte(js), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("--json agenda = %s (%v)", js, err)
	}
	c := srv.Dial(t)
	ev, err := c.Calendar().Event(ctx, &api.CalendarEventParams{ID: rows[0].EventID})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Organizer != nil || ev.UID != "flight@airline.example" {
		t.Errorf("the added event = %+v", ev)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "cal", "add-from-message", id); err == nil {
		t.Error("a second add succeeded")
	}
}
