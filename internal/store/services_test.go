package store

import (
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

func TestServices(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("one@mailtest.test"))
	b := insertAccount(t, d, sampleAccount("two@mailtest.test"))
	*events = nil
	set := func(id int64, s api.ServiceKind, on bool, url string) error {
		return d.Tx(ctx, func(tx *Tx) error { return tx.SetService(ctx, id, s, on, url) })
	}
	for _, s := range []struct {
		id  int64
		svc api.ServiceKind
		url string
	}{
		{a.ID, api.ServiceKindTasks, "https://tasks.test/"},
		{a.ID, api.ServiceKindContacts, "https://dav.test/contacts/"},
		{b.ID, api.ServiceKindCalendar, "https://dav.test/calendars/"},
	} {
		if err := set(s.id, s.svc, true, s.url); err != nil {
			t.Fatal(err)
		}
	}
	if len(*events) != 3 {
		t.Errorf("events = %d, want an account.changed per change", len(*events))
	}
	if err := set(999, api.ServiceKindContacts, true, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown account: %v", err)
	}

	got, err := d.Services(ctx, a.ID)
	if err != nil || len(got) != 2 || got[0].Service != api.ServiceKindContacts || got[1].Service != api.ServiceKindTasks {
		t.Fatalf("Services(a) = %+v, %v", got, err)
	}
	all, err := d.Services(ctx, 0)
	if err != nil || len(all) != 3 || all[2].AccountID != b.ID {
		t.Fatalf("Services(0) = %+v, %v", all, err)
	}

	*events = nil
	at := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	synced := func(errText string) {
		t.Helper()
		if err := d.Tx(ctx, func(tx *Tx) error {
			return tx.ServiceSynced(ctx, a.ID, api.ServiceKindContacts, at, errText)
		}); err != nil {
			t.Fatal(err)
		}
	}
	synced("")
	synced("offline")
	synced("offline")
	got, _ = d.Services(ctx, a.ID)
	if got[0].LastSyncAt == nil || !got[0].LastSyncAt.Equal(at) || got[0].LastError != "offline" {
		t.Errorf("after passes: %+v", got[0])
	}
	if len(*events) != 1 {
		t.Errorf("events = %d, want one, when the error first appeared", len(*events))
	}

	// Turning off keeps the URL and the last pass; a new URL forgets them.
	if err := set(a.ID, api.ServiceKindContacts, false, "https://dav.test/contacts/"); err != nil {
		t.Fatal(err)
	}
	got, _ = d.Services(ctx, a.ID)
	if got[0].Enabled || got[0].LastSyncAt == nil {
		t.Errorf("off: %+v", got[0])
	}
	if err := set(a.ID, api.ServiceKindContacts, true, "https://other.test/"); err != nil {
		t.Fatal(err)
	}
	got, _ = d.Services(ctx, a.ID)
	if !got[0].Enabled || got[0].LastSyncAt != nil || got[0].LastError != "" || got[0].URL != "https://other.test/" {
		t.Errorf("moved: %+v", got[0])
	}
}
