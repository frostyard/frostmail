package main

// CONTRACT TEST for task card T-0090 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

const adaCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nORG:Engines\r\n" +
	"EMAIL;TYPE=HOME:ada@example.com\r\nTEL;TYPE=CELL:+44 20 7946 0000\r\nNOTE:Countess\r\nEND:VCARD\r\n"

const alanCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:alan\r\nFN:Alan Turing\r\nN:Turing;Alan;;;\r\n" +
	"EMAIL:alan@example.com\r\nEND:VCARD\r\n"

func ics(lines ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\n" +
		strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

// pimIDs are the fixture's IDs.
type pimIDs struct {
	acct, book, work, home, tasks, errands int64
	ada, report, charts, done, milk        int64
}

// pimFixture stores an account with an address book (Ada, Alan), a shown
// calendar Work and a hidden one Home, and two Google task lists.
func pimFixture(t *testing.T) (*rpctest.Server, pimIDs) {
	t.Helper()
	srv := rpctest.Start(t)
	ctx := t.Context()
	var ids pimIDs
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "imap.example", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: "ann@gmail.example",
			Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		ids.acct = acct.ID
		for _, s := range []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks} {
			if err := tx.SetService(ctx, acct.ID, s, true, "https://dav.example/"); err != nil {
				return err
			}
		}
		books, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindAddressbook, []store.RemoteCollection{{Href: "/ab/", Name: "Contacts"}})
		if err != nil {
			return err
		}
		ids.book = books[0].ID
		for _, c := range []struct {
			href, raw string
			idx       store.ContactIndex
			id        *int64
		}{
			{"/ab/ada.vcf", adaCard, store.ContactIndex{DisplayName: "Ada Lovelace", SortKey: "lovelace ada", GivenName: "Ada",
				FamilyName: "Lovelace", Organization: "Engines", Emails: []store.ContactEmail{{Email: "ada@example.com", Label: "home"}}}, &ids.ada},
			{"/ab/alan.vcf", alanCard, store.ContactIndex{DisplayName: "Alan Turing", SortKey: "turing alan", GivenName: "Alan",
				FamilyName: "Turing", Emails: []store.ContactEmail{{Email: "alan@example.com"}}}, nil},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: ids.book, Href: c.href, ETag: `"1"`, Kind: store.ObjectVCard, Raw: []byte(c.raw)})
			if err != nil {
				return err
			}
			if err := tx.IndexContact(ctx, id, c.idx); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		cals, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar, []store.RemoteCollection{
			{Href: "/work/", Name: "Work"}, {Href: "/home/", Name: "Home", ReadOnly: true}})
		if err != nil {
			return err
		}
		ids.work, ids.home = cals[0].ID, cals[1].ID
		if _, err := tx.UpdateCollection(ctx, ids.home, new(bool), false); err != nil {
			return err
		}
		from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, o := range []struct {
			col  int64
			href string
			raw  []byte
		}{
			{ids.work, "/work/standup.ics", ics("UID:standup", "DTSTART:20261015T090000Z", "DTEND:20261015T091500Z", "SUMMARY:Standup", "LOCATION:Room 4")},
			{ids.work, "/work/review.ics", ics("UID:review", "DTSTART:20261015T140000Z", "DTEND:20261015T153000Z", "SUMMARY:Review")},
			{ids.work, "/work/holiday.ics", ics("UID:holiday", "DTSTART;VALUE=DATE:20261016", "DTEND;VALUE=DATE:20261017", "SUMMARY:Holiday")},
			{ids.work, "/work/off.ics", ics("UID:off", "DTSTART:20261016T100000Z", "DTEND:20261016T110000Z", "SUMMARY:Offsite", "STATUS:CANCELLED")},
			{ids.home, "/home/gym.ics", ics("UID:gym", "DTSTART:20261015T180000Z", "DTEND:20261015T190000Z", "SUMMARY:Gym")},
		} {
			if _, err := pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: o.col, Href: o.href, ETag: `"1"`,
				Kind: store.ObjectVEvent, Raw: o.raw}, calendar.Options{Local: time.UTC}, from, to); err != nil {
				return err
			}
		}
		if err := tx.SetInstanceWindow(ctx, from, to); err != nil {
			return err
		}
		lists, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindTasklist, []store.RemoteCollection{
			{Href: "L1", Name: "Tasks"}, {Href: "L2", Name: "Errands"}})
		if err != nil {
			return err
		}
		ids.tasks, ids.errands = lists[0].ID, lists[1].ID
		for _, r := range []struct {
			col int64
			row store.TaskRow
			id  *int64
		}{
			{ids.tasks, store.TaskRow{UID: "t1", Title: "Report", Position: "00001", Due: "2026-10-10"}, &ids.report},
			{ids.tasks, store.TaskRow{UID: "t2", ParentUID: "t1", Title: "Charts", Position: "00001"}, &ids.charts},
			{ids.tasks, store.TaskRow{UID: "t3", Title: "Done", Position: "00002", Due: "2026-10-01", Completed: true}, &ids.done},
			{ids.errands, store.TaskRow{UID: "t4", Title: "Buy oat milk"}, &ids.milk},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: r.col, Href: r.row.UID, Kind: store.ObjectGTask, UID: r.row.UID,
				Raw: []byte(`{"id":"` + r.row.UID + `","title":"` + r.row.Title + `"}`)})
			if err != nil {
				return err
			}
			if err := tx.IndexTask(ctx, id, r.row); err != nil {
				return err
			}
			*r.id = id
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	people, err := srv.DB.People(ctx, "")
	if err != nil || len(people) == 0 {
		t.Fatalf("people = %+v, %v", people, err)
	}
	ids.ada = people[0].ID
	return srv, ids
}

var cellGap = regexp.MustCompile(`\s{2,}`)

// cells splits each output line into its columns.
func cells(out string) [][]string {
	var rows [][]string
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		rows = append(rows, cellGap.Split(strings.TrimRight(line, " "), -1))
	}
	return rows
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runMailctl(t, args...)
	if err != nil {
		t.Fatalf("mailctl %q: %v\n%s", args, err, out)
	}
	return out
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func TestPeopleCommands(t *testing.T) {
	srv, ids := pimFixture(t)
	got := cells(mustRun(t, "--socket", srv.Socket, "people", "ls"))
	want := [][]string{
		{"ID", "NAME", "EMAIL", "ORGANIZATION"},
		{itoa64(ids.ada), "Ada Lovelace", "ada@example.com", "Engines"},
		{got[2][0], "Alan Turing", "alan@example.com", "-"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("people ls = %q", got)
	}
	if got := cells(mustRun(t, "--socket", srv.Socket, "people", "ls", "turing")); len(got) != 2 || got[1][1] != "Alan Turing" {
		t.Errorf("people ls turing = %q", got)
	}
	if got := mustRun(t, "--socket", srv.Socket, "people", "ls", "nobody"); got != "no people\n" {
		t.Errorf("no match = %q", got)
	}
	show := mustRun(t, "--socket", srv.Socket, "people", "show", itoa64(ids.ada))
	want2 := "Ada Lovelace\nOrganization: Engines\nEmail: ada@example.com (home)\nPhone: +44 20 7946 0000 (mobile)\nNote: Countess\n"
	if show != want2 {
		t.Errorf("people show =\n%s\nwant\n%s", show, want2)
	}
	js := mustRun(t, "--socket", srv.Socket, "--json", "people", "ls")
	var list []api.PersonSummary
	if err := json.Unmarshal([]byte(js), &list); err != nil || len(list) != 2 {
		t.Errorf("--json people ls = %s (%v)", js, err)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "people", "show", "99999"); err == nil {
		t.Error("an unknown person printed")
	}
}

func TestCalendarCommands(t *testing.T) {
	srv, ids := pimFixture(t)
	got := cells(mustRun(t, "--socket", srv.Socket, "cal", "ls"))
	want := [][]string{
		{"ID", "ACCOUNT", "NAME", "SHOWN", "READ-ONLY"},
		{itoa64(ids.work), itoa64(ids.acct), "Work", "yes", "no"},
		{itoa64(ids.home), itoa64(ids.acct), "Home", "no", "yes"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("cal ls = %q", got)
	}
	agenda := mustRun(t, "--socket", srv.Socket, "cal", "agenda", "--from", "2026-10-15", "--days", "2", "--zone", "UTC")
	want2 := "2026-10-15 Thu\n" +
		"  09:00–09:15  Standup (Room 4)\n" +
		"  14:00–15:30  Review\n" +
		"2026-10-16 Fri\n" +
		"  all day      Holiday\n" +
		"  10:00–11:00  Offsite [cancelled]\n"
	if agenda != want2 {
		t.Errorf("agenda =\n%s\nwant\n%s", agenda, want2)
	}
	berlin := mustRun(t, "--socket", srv.Socket, "cal", "agenda", "--from", "2026-10-15", "--days", "1", "--zone", "Europe/Berlin")
	if !strings.Contains(berlin, "  11:00–11:15  Standup (Room 4)\n") {
		t.Errorf("agenda in Berlin =\n%s", berlin)
	}
	if got := mustRun(t, "--socket", srv.Socket, "cal", "agenda", "--from", "2026-12-01", "--days", "1", "--zone", "UTC"); got != "no events\n" {
		t.Errorf("an empty day = %q", got)
	}
	for _, bad := range [][]string{{"--from", "soon"}, {"--days", "0"}, {"--zone", "Mars/Olympus"}} {
		if _, err := runMailctl(t, append([]string{"--socket", srv.Socket, "cal", "agenda"}, bad...)...); err == nil {
			t.Errorf("agenda %q succeeded", bad)
		}
	}
}

func TestTasksCommands(t *testing.T) {
	srv, ids := pimFixture(t)
	got := cells(mustRun(t, "--socket", srv.Socket, "tasks", "ls"))
	want := [][]string{
		{"ID", "DONE", "DUE", "LIST", "TITLE"},
		{itoa64(ids.report), "-", "2026-10-10", "Tasks", "Report"},
		{itoa64(ids.charts), "-", "-", "Tasks", "↳ Charts"},
		{itoa64(ids.milk), "-", "-", "Errands", "Buy oat milk"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("tasks ls = %q", got)
	}
	all := cells(mustRun(t, "--socket", srv.Socket, "tasks", "ls", "--all", "--list", itoa64(ids.tasks)))
	if len(all) != 4 || !slices.Equal(all[3], []string{itoa64(ids.done), "x", "2026-10-01", "Tasks", "Done"}) {
		t.Errorf("tasks ls --all --list = %q", all)
	}
	if due := cells(mustRun(t, "--socket", srv.Socket, "tasks", "ls", "--due-before", "2026-10-11")); len(due) != 2 || due[1][4] != "Report" {
		t.Errorf("tasks ls --due-before = %q", due)
	}

	out := mustRun(t, "--socket", srv.Socket, "tasks", "add", "--list", itoa64(ids.errands), "--due", "2026-10-12", "--notes", "Oat", "Call", "Ann")
	var made int64
	if n, err := scanTask(out); err != nil {
		t.Fatalf("tasks add printed %q", out)
	} else {
		made = n
	}
	task, err := srv.DB.Task(t.Context(), made)
	if err != nil || task.Title != "Call Ann" || task.Due != "2026-10-12" || task.Notes != "Oat" || task.ListID != ids.errands {
		t.Errorf("the new task = %+v, %v", task, err)
	}
	sub := mustRun(t, "--socket", srv.Socket, "tasks", "add", "--parent", itoa64(ids.report), "Tables")
	if n, err := scanTask(sub); err != nil {
		t.Errorf("subtask: %q", sub)
	} else if row, _ := srv.DB.Task(t.Context(), n); row.ParentID != ids.report {
		t.Errorf("the subtask = %+v", row)
	}

	if out := mustRun(t, "--socket", srv.Socket, "tasks", "done", itoa64(ids.milk)); out != "" {
		t.Errorf("done printed %q", out)
	}
	if row, _ := srv.DB.Task(t.Context(), ids.milk); !row.Completed {
		t.Error("tasks done did not complete it")
	}
	mustRun(t, "--socket", srv.Socket, "tasks", "done", "--undo", itoa64(ids.milk))
	if row, _ := srv.DB.Task(t.Context(), ids.milk); row.Completed {
		t.Error("tasks done --undo did not reopen it")
	}
	mustRun(t, "--socket", srv.Socket, "tasks", "rm", itoa64(made))
	if _, err := srv.DB.Task(t.Context(), made); err == nil {
		t.Error("tasks rm left the task")
	}

	js := mustRun(t, "--socket", srv.Socket, "--json", "tasks", "ls")
	var tasks []api.Task
	if err := json.Unmarshal([]byte(js), &tasks); err != nil || len(tasks) < 3 {
		t.Errorf("--json tasks ls = %s (%v)", js, err)
	}
	for _, bad := range [][]string{{"tasks", "add"}, {"tasks", "add", "--due", "soon", "x"}, {"tasks", "done", "abc"}, {"tasks", "done", "99999"}} {
		if _, err := runMailctl(t, append([]string{"--socket", srv.Socket}, bad...)...); err == nil {
			t.Errorf("%q succeeded", bad)
		}
	}
	if got := mustRun(t, "--socket", srv.Socket, "tasks", "ls", "--due-before", "2000-01-01"); got != "no tasks\n" {
		t.Errorf("no tasks = %q", got)
	}
}

// scanTask reads "task N" from tasks add.
func scanTask(out string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(strings.TrimSpace(out), "task %d", &n)
	return n, err
}
