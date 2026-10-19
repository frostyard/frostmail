package lodge_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// maxText is the tools' cut for a text, 32 KiB.
const maxText = 32 << 10

// seedPIMLimits stores one account whose calendar holds a chain of 52
// timed events on 2026-10-20, each overlapping the next only (51 clashes),
// a chain of 51 on the 21st (50 clashes), and on the 22nd an event with
// 100 attendees and a 32 KiB description and one with 101 attendees and a
// longer one; whose task list holds 101 open tasks, two of them with notes
// of 32 KiB and one byte more; and whose address book holds 51 people, two
// of them with notes of those lengths.
func seedPIMLimits(t *testing.T) (srv *rpctest.Server, ids map[string]int64) {
	t.Helper()
	srv = rpctest.Start(t)
	ctx := t.Context()
	ids = map[string]int64{}
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test", DisplayName: "Me",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		s := pimStore{t, tx}
		s.services(a.ID)
		cal := s.collection(a.ID, api.CollectionKindCalendar, "/cal/", "Cal")
		list := s.collection(a.ID, api.CollectionKindTasklist, "L", "Tasks")
		book := s.collection(a.ID, api.CollectionKindAddressbook, "/ab/", "Book")
		chain := func(day string, n int) {
			base, _ := time.Parse(time.DateOnly, day)
			for i := range n {
				start := base.Add(time.Duration(15*i) * time.Minute)
				s.event(cal, "me@a.test", fmt.Sprintf("/cal/%s-%d.ics", day, i), vevent(fmt.Sprintf("UID:%s-%d", day, i),
					"DTSTART:"+start.Format("20060102T150405Z"), "DTEND:"+start.Add(20*time.Minute).Format("20060102T150405Z"),
					fmt.Sprintf("SUMMARY:Step %d", i)))
			}
		}
		chain("2026-10-20", 52)
		chain("2026-10-21", 51)
		for _, n := range []int{100, 101} {
			lines := []string{fmt.Sprintf("UID:crowd-%d", n), "DTSTART:20261022T090000Z", "DTEND:20261022T100000Z",
				fmt.Sprintf("SUMMARY:Crowd %d", n), "DESCRIPTION:" + strings.Repeat("d", maxText+n-100),
				"ORGANIZER:mailto:org@a.test"}
			for i := range n {
				lines = append(lines, fmt.Sprintf("ATTENDEE:mailto:p%d@a.test", i))
			}
			s.event(cal, "me@a.test", fmt.Sprintf("/cal/crowd-%d.ics", n), vevent(lines...))
		}
		for i := range 101 {
			row := store.TaskRow{UID: fmt.Sprintf("t%d", i), Title: fmt.Sprintf("Task %03d", i), Position: fmt.Sprintf("%05d", i)}
			switch i {
			case 0:
				row.Notes = strings.Repeat("n", maxText)
			case 1:
				row.Notes = strings.Repeat("n", maxText+1)
			}
			s.task(list, row)
		}
		for i := range 51 {
			note := ""
			switch i {
			case 0:
				note = strings.Repeat("c", maxText)
			case 1:
				note = strings.Repeat("c", maxText+1)
			}
			s.contact(book, fmt.Sprintf("p%d", i), fmt.Sprintf("Person %02d", i), "", fmt.Sprintf("p%02d@a.test", i), note)
		}
		s.done()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	local := srv.Dial(t)
	occ, err := local.Calendar().Range(ctx, &api.CalendarRangeParams{From: "2026-10-22", To: "2026-10-23", TimeZone: new("UTC")})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range occ {
		ids[o.Summary] = o.EventID
	}
	tasks, err := local.Tasks().List(ctx, &api.TasksListParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		ids[task.Title] = task.ID
	}
	people, err := local.People().List(ctx, &api.PeopleListParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range people {
		ids[p.DisplayName] = p.ID
	}
	if len(tasks) != 101 || len(people) != 51 || ids["Crowd 100"] == 0 || ids["Crowd 101"] == 0 {
		t.Fatalf("seeded %d tasks, %d people, events %v", len(tasks), len(people), occ)
	}
	return srv, ids
}

// TestThePIMToolsLimits: every limit of the calendar, task and contact
// tools at its value and one past it (docs/specs/lodge-app.md, Calendar,
// tasks and contacts).
func TestThePIMToolsLimits(t *testing.T) {
	srv, ids := seedPIMLimits(t)
	cs, err := mcpClient(t, lodgeAt(t, srv), "claude", "account=*")
	if err != nil {
		t.Fatal(err)
	}
	var events struct {
		Total      int        `json:"total"`
		Events     []struct{} `json:"events"`
		Clashes    []struct{} `json:"clashes"`
		ClashCount int        `json:"clash_count"`
	}
	for _, c := range []struct {
		from              string
		clashes, wantKept int
	}{{"2026-10-20", 51, 50}, {"2026-10-21", 50, 50}} {
		if call(t, cs, "list_events", map[string]any{"from": c.from, "time_zone": "UTC"}, &events) ||
			events.ClashCount != c.clashes || len(events.Clashes) != c.wantKept {
			t.Errorf("list_events %s = %d of %d clashes, want %d of %d", c.from, len(events.Clashes), events.ClashCount, c.wantKept, c.clashes)
		}
	}
	// limit 0 is the default, as an omitted one is.
	for limit, want := range map[int]int{0: 100, 1: 1, 200: 105} {
		args := map[string]any{"from": "2026-10-20", "to": "2026-10-23", "time_zone": "UTC"}
		if limit != 0 {
			args["limit"] = limit
		}
		if call(t, cs, "list_events", args, &events) || events.Total != 105 || len(events.Events) != want {
			t.Errorf("list_events limit %d = %d of %d, want %d of 105", limit, len(events.Events), events.Total, want)
		}
	}
	if call(t, cs, "list_events", map[string]any{"from": "2026-10-01", "to": "2026-11-01"}, &events) {
		t.Error("list_events over 31 days answered an error")
	}
	for _, args := range []map[string]any{
		{"from": "2026-10-01", "to": "2026-11-02"},
		{"from": "2026-10-20", "to": "2026-10-20"},
		{"from": "2026-10-20", "limit": -1},
		{"from": "2026-10-20", "limit": 201},
	} {
		if !call(t, cs, "list_events", args, &events) {
			t.Errorf("list_events %v answered no error", args)
		}
	}

	var event struct {
		Description   string     `json:"description"`
		Truncated     bool       `json:"truncated"`
		Attendees     []struct{} `json:"attendees"`
		AttendeeCount int        `json:"attendee_count"`
	}
	if call(t, cs, "read_event", map[string]any{"event_id": ids["Crowd 100"]}, &event) || event.AttendeeCount != 100 ||
		len(event.Attendees) != 100 || event.Truncated || len(event.Description) != maxText {
		t.Errorf("read_event of 100 attendees = %d of %d, description %d bytes (cut %v); want all, whole",
			len(event.Attendees), event.AttendeeCount, len(event.Description), event.Truncated)
	}
	if call(t, cs, "read_event", map[string]any{"event_id": ids["Crowd 101"]}, &event) || event.AttendeeCount != 101 ||
		len(event.Attendees) != 100 || !event.Truncated || len(event.Description) != maxText {
		t.Errorf("read_event of 101 attendees = %d of %d, description %d bytes (cut %v); want 100, cut at 32 KiB",
			len(event.Attendees), event.AttendeeCount, len(event.Description), event.Truncated)
	}

	var tasks struct {
		Total int        `json:"total"`
		Tasks []struct{} `json:"tasks"`
	}
	for limit, want := range map[int]int{0: 50, 1: 1, 100: 100} {
		args := map[string]any{}
		if limit != 0 {
			args["limit"] = limit
		}
		if call(t, cs, "list_tasks", args, &tasks) || tasks.Total != 101 || len(tasks.Tasks) != want {
			t.Errorf("list_tasks limit %d = %d of %d, want %d of 101", limit, len(tasks.Tasks), tasks.Total, want)
		}
	}
	for _, limit := range []int{-1, 101} {
		if !call(t, cs, "list_tasks", map[string]any{"limit": limit}, &tasks) {
			t.Errorf("list_tasks limit %d answered no error", limit)
		}
	}
	var task struct {
		Notes     string `json:"notes"`
		Truncated bool   `json:"truncated"`
	}
	for title, cut := range map[string]bool{"Task 000": false, "Task 001": true} {
		if call(t, cs, "read_task", map[string]any{"task_id": ids[title]}, &task) || task.Truncated != cut || len(task.Notes) != maxText {
			t.Errorf("read_task of %s = %d bytes of notes (cut %v), want %d (cut %v)", title, len(task.Notes), task.Truncated, maxText, cut)
		}
	}

	var found struct {
		Total  int        `json:"total"`
		People []struct{} `json:"people"`
	}
	for limit, want := range map[int]int{0: 20, 1: 1, 50: 50} {
		args := map[string]any{"query": "person"}
		if limit != 0 {
			args["limit"] = limit
		}
		if call(t, cs, "search_contacts", args, &found) || found.Total != 51 || len(found.People) != want {
			t.Errorf("search_contacts limit %d = %d of %d, want %d of 51", limit, len(found.People), found.Total, want)
		}
	}
	for _, q := range []string{"p", "person" + strings.Repeat(" ", 250)} {
		if call(t, cs, "search_contacts", map[string]any{"query": q}, &found) || found.Total != 51 {
			t.Errorf("search_contacts of a %d-byte query = %d people, want 51", len(q), found.Total)
		}
	}
	for _, args := range []map[string]any{
		{"query": "person", "limit": -1},
		{"query": "person", "limit": 51},
		{"query": ""},
		{"query": "   "},
		{"query": "person" + strings.Repeat(" ", 251)},
	} {
		if !call(t, cs, "search_contacts", args, &found) {
			t.Errorf("search_contacts %q, limit %v answered no error", args["query"], args["limit"])
		}
	}
	var contact struct {
		Contacts []struct {
			Note      string `json:"note"`
			Truncated bool   `json:"truncated"`
		} `json:"contacts"`
	}
	for name, cut := range map[string]bool{"Person 00": false, "Person 01": true} {
		if call(t, cs, "read_contact", map[string]any{"person_id": ids[name]}, &contact) || len(contact.Contacts) != 1 ||
			contact.Contacts[0].Truncated != cut || len(contact.Contacts[0].Note) != maxText {
			t.Errorf("read_contact of %s = %+v, want a note of 32 KiB (cut %v)", name, contact, cut)
		}
	}
}
