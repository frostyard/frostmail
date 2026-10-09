//go:build integration

package engine_test

// Runs against Radicale in the frostmail-mailtest container (make
// engine-it), whose users' address books and calendars are empty in the
// clean snapshot. As test3@mailtest.test, contacts reach People from
// another client and from Add to Contacts, and verify then finds both
// sides the same (plan 0007, Phase 2); as test4, calendars (Phase 3).

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/reminders"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestRadicaleContacts(t *testing.T) {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	const user, password = "test3@mailtest.test", "frostmail-test"
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the test container's certificate
	}}
	srv := rpctest.StartWith(t, rpctest.Options{PIM: &pimsync.Config{HTTP: insecure, Interval: time.Hour}})
	c := srv.Dial(t)
	ctx := t.Context()
	p := createParams(user)
	p.IMAP.Host, p.SMTP.Host = host, host
	a, err := c.Account().Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: password}); err != nil {
		t.Fatal(err)
	}
	start := "https://" + host + ":5232/"
	svc, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: a.ID, Service: api.ServiceKindContacts,
		Enabled: true, URL: &start})
	if err != nil || !svc.Enabled {
		t.Fatalf("setService = %+v, %v", svc, err)
	}
	pass := func() {
		t.Helper()
		if err := srv.PIM.Pass(ctx, a.ID); err != nil {
			t.Fatalf("Pass: %v", err)
		}
	}
	pass()
	cols, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &a.ID})
	if err != nil || len(cols) != 1 || cols[0].Name != "Contacts" {
		t.Fatalf("collections = %+v, %v", cols, err)
	}

	// Another client adds a contact; it reaches People.
	other := davClient(t, ctx, start, user, password, insecure, davx.AddressBooks)
	book := "/" + strings.ReplaceAll(user, "@", "%40") + "/contacts/"
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:radicale-ada\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n" +
		"EMAIL;TYPE=INTERNET:ada@example.com\r\nEND:VCARD\r\n"
	if _, err := other.Put(ctx, book+"radicale-ada.vcf", []byte(card), davx.AddressBooks, ""); err != nil {
		t.Fatal(err)
	}
	pass()
	people, err := c.People().List(ctx, &api.PeopleListParams{})
	if err != nil || len(people) != 1 || people[0].DisplayName != "Ada Lovelace" {
		t.Fatalf("people after another client's contact = %+v, %v", people, err)
	}

	// Add to Contacts reaches the server.
	if _, err := c.People().Add(ctx, &api.PeopleAddParams{Email: "grace@navy.example", Name: ptr("Grace Hopper")}); err != nil {
		t.Fatal(err)
	}
	pass()
	list, err := other.List(ctx, book)
	if err != nil || len(list) != 2 {
		t.Fatalf("server objects = %+v, %v", list, err)
	}
	var found bool
	for _, o := range list {
		obj, err := other.Get(ctx, o.Href)
		if err != nil {
			t.Fatal(err)
		}
		found = found || strings.Contains(string(obj.Data), "Grace Hopper")
	}
	if !found {
		t.Error("the added contact is not on the server")
	}
	checks, err := srv.PIM.Verify(ctx, a.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Server != 2 || checks[0].Local != 2 {
		t.Errorf("verify = %+v, %v", checks, err)
	}

	// Another client removes Ada; she leaves People.
	if err := other.Delete(ctx, book+"radicale-ada.vcf", ""); err != nil {
		t.Fatal(err)
	}
	pass()
	if people, _ := c.People().List(ctx, &api.PeopleListParams{}); len(people) != 1 || people[0].DisplayName != "Grace Hopper" {
		t.Errorf("people after the delete = %+v", people)
	}
}

// TestRadicaleCalendar: Phase 3's exit (plan 0007). A series with an
// override, an all-day event, an event in a Windows-named zone and a series
// across a DST change, put on Radicale by another client as
// test4@mailtest.test, come back at the right times, and the series'
// alarm fires on time.
func TestRadicaleCalendar(t *testing.T) {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	const user, password = "test4@mailtest.test", "frostmail-test"
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the test container's certificate
	}}
	var mu sync.Mutex
	now := time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC)
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	srv := rpctest.StartWith(t, rpctest.Options{PIM: &pimsync.Config{HTTP: insecure, Interval: time.Hour, Local: time.UTC}, Now: clock})
	c := srv.Dial(t)
	ctx := t.Context()
	p := createParams(user)
	p.IMAP.Host, p.SMTP.Host = host, host
	a, err := c.Account().Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: password}); err != nil {
		t.Fatal(err)
	}
	start := "https://" + host + ":5232/"
	if _, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: a.ID, Service: api.ServiceKindCalendar,
		Enabled: true, URL: &start}); err != nil {
		t.Fatal(err)
	}

	other := davClient(t, ctx, start, user, password, insecure, davx.Calendars)
	cal := "/" + strings.ReplaceAll(user, "@", "%40") + "/calendar/"
	put := func(name string, lines ...string) {
		t.Helper()
		raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Frostmail//IT//EN\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCALENDAR\r\n"
		if _, err := other.Put(ctx, cal+name+".ics", []byte(raw), davx.Calendars, ""); err != nil {
			t.Fatal(err)
		}
	}
	put("standup",
		"BEGIN:VEVENT", "UID:it-standup", "DTSTAMP:20261001T000000Z", "SUMMARY:Standup",
		"DTSTART;TZID=Europe/Berlin:20261020T090000", "DTEND;TZID=Europe/Berlin:20261020T091500", "RRULE:FREQ=DAILY;COUNT=10",
		"BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:Standup", "TRIGGER:-PT10M", "END:VALARM", "END:VEVENT",
		"BEGIN:VEVENT", "UID:it-standup", "DTSTAMP:20261001T000000Z", "SUMMARY:Standup (late)",
		"RECURRENCE-ID;TZID=Europe/Berlin:20261022T090000",
		"DTSTART;TZID=Europe/Berlin:20261022T110000", "DTEND;TZID=Europe/Berlin:20261022T111500", "END:VEVENT")
	put("holiday", "BEGIN:VEVENT", "UID:it-holiday", "DTSTAMP:20261001T000000Z", "SUMMARY:Holiday",
		"DTSTART;VALUE=DATE:20261023", "DTEND;VALUE=DATE:20261024", "END:VEVENT")
	put("outlook",
		"BEGIN:VTIMEZONE", "TZID:Eastern Standard Time",
		"BEGIN:STANDARD", "DTSTART:16010101T020000", "TZOFFSETFROM:-0400", "TZOFFSETTO:-0500",
		"RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=11", "END:STANDARD",
		"BEGIN:DAYLIGHT", "DTSTART:16010101T020000", "TZOFFSETFROM:-0500", "TZOFFSETTO:-0400",
		"RRULE:FREQ=YEARLY;BYDAY=2SU;BYMONTH=3", "END:DAYLIGHT", "END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:it-outlook", "DTSTAMP:20261001T000000Z", "SUMMARY:Outlook call",
		"DTSTART;TZID=Eastern Standard Time:20261021T140000", "DTEND;TZID=Eastern Standard Time:20261021T150000", "END:VEVENT")
	put("dst", "BEGIN:VEVENT", "UID:it-dst", "DTSTAMP:20261001T000000Z", "SUMMARY:Across DST",
		"DTSTART;TZID=America/New_York:20261030T090000", "DTEND;TZID=America/New_York:20261030T093000",
		"RRULE:FREQ=DAILY;COUNT=4", "END:VEVENT")
	if err := srv.PIM.Pass(ctx, a.ID); err != nil {
		t.Fatalf("Pass: %v", err)
	}

	occ, err := c.Calendar().Range(ctx, &api.CalendarRangeParams{From: "2026-10-20", To: "2026-11-03", TimeZone: ptr("UTC")})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range occ {
		if o.AllDay {
			got = append(got, o.StartDate+" "+o.Summary)
		} else {
			got = append(got, o.Start.UTC().Format("01-02T15:04")+" "+o.Summary)
		}
	}
	want := []string{
		"10-20T07:00 Standup", "10-21T07:00 Standup", "10-21T18:00 Outlook call", "10-22T09:00 Standup (late)",
		"2026-10-23 Holiday", "10-23T07:00 Standup", "10-24T07:00 Standup",
		// Berlin leaves summer time on October 25: 9:00 is 8:00 UTC.
		"10-25T08:00 Standup", "10-26T08:00 Standup", "10-27T08:00 Standup", "10-28T08:00 Standup", "10-29T08:00 Standup",
		// New York leaves it on November 1.
		"10-30T13:00 Across DST", "10-31T13:00 Across DST", "11-01T14:00 Across DST", "11-02T14:00 Across DST",
	}
	if !slices.Equal(got, want) {
		t.Errorf("occurrences:\n%q\nwant\n%q", got, want)
	}

	// The standup's alarm, ten minutes before 9:00 Berlin, fires at 6:50 UTC.
	sched := reminders.New(reminders.Config{DB: srv.DB, Now: clock, Local: time.UTC,
		List: func(ctx context.Context) ([]api.Reminder, error) {
			return c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
		}})
	if err := sched.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ := c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{}); len(list) != 0 {
		t.Fatalf("reminders at 6:00 = %+v", list)
	}
	mu.Lock()
	now = time.Date(2026, 10, 20, 6, 50, 0, 0, time.UTC)
	mu.Unlock()
	if err := sched.Check(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
	if err != nil || len(list) != 1 || list[0].Summary != "Standup" ||
		!list[0].DueAt.Equal(time.Date(2026, 10, 20, 6, 50, 0, 0, time.UTC)) {
		t.Errorf("reminders at 6:50 = %+v, %v", list, err)
	}
}

func davClient(t *testing.T, ctx context.Context, start, user, password string, hc *http.Client, kind davx.Kind) *davx.Client {
	t.Helper()
	opts := davx.Options{HTTP: hc, Authorization: func(context.Context) (string, error) {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
	}}
	home, err := davx.Discover(ctx, start, kind, opts)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := davx.New(home, opts)
	if err != nil {
		t.Fatal(err)
	}
	return cl
}
