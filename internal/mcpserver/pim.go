package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The calendar, task and contact read tools adapt maild's account,
// calendar, tasks and people domains (docs/specs/lodge-app.md, Calendar,
// tasks and contacts; ADR-0042). maild's engine keeps another account's
// collections, events, tasks and contacts from the agent.

// Limits on what the calendar, task and contact tools take and return.
const (
	maxEventDays    = 31
	maxEvents       = 200
	defaultEvents   = 100
	maxClashes      = 50
	maxAttendees    = 100
	maxTasks        = 100
	defaultTasks    = 50
	maxPeople       = 50
	defaultPeople   = 20
	maxContactQuery = 256
)

// addPIMTools adds the calendar, task and contact read tools.
func addPIMTools(s *sdk.Server, t *tools) {
	reads := &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
	sdk.AddTool(s, &sdk.Tool{Name: "list_collections", Annotations: reads, Description: `List the calendars,
task lists and address books of the accounts this token reaches: their IDs,
accounts, names, what they hold, and whether they are read-only or their
account's default. ` + Notice}, t.listCollections)
	sdk.AddTool(s, &sdk.Tool{Name: "list_events", Annotations: reads, Description: `List the calendar's
occurrences on a range of days, earliest first: one day by default, at
most 31. Each has its event's ID, summary, place, times and the user's
answer, and whether it counts as busy. clashes pairs the busy timed
occurrences that overlap. ` + Notice}, t.listEvents)
	sdk.AddTool(s, &sdk.Tool{Name: "read_event", Annotations: reads, Description: `Read an event, or one
occurrence of a series, by its IDs from list_events: its description (cut
at 32 KiB), times, recurrence, organizer and attendees (the first 100)
with their answers. ` + Notice}, t.readEvent)
	sdk.AddTool(s, &sdk.Tool{Name: "list_tasks", Annotations: reads, Description: `List the tasks, list by
list: those due today or before, overdue, undated, or all of them,
optionally in one list. Completed tasks are left out unless asked for. ` + Notice}, t.listTasks)
	sdk.AddTool(s, &sdk.Tool{Name: "read_task", Annotations: reads, Description: `Read a task by its ID from
list_tasks: its notes (cut at 32 KiB), due date, list, and the message it
was made from, which read_thread reads. ` + Notice}, t.readTask)
	sdk.AddTool(s, &sdk.Tool{Name: "search_contacts", Annotations: reads, Description: `Find people in the
contacts whose name, organization or email address has words starting with
query, by name. ` + Notice}, t.searchContacts)
	sdk.AddTool(s, &sdk.Tool{Name: "read_contact", Annotations: reads, Description: `Read a person by their ID
from search_contacts: each of their contacts, with its email addresses,
phones, postal addresses, URLs, birthday and note (cut at 32 KiB). ` + Notice}, t.readContact)
}

// zoneOf loads the zone an agent names, or maild's own for none.
func zoneOf(name string) (*time.Location, string, error) {
	if name == "" {
		return time.Local, time.Local.String(), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil || name == "Local" {
		return nil, "", fmt.Errorf("time_zone %q is not an IANA time zone", name)
	}
	return loc, name, nil
}

type collectionsIn struct {
	Kind string `json:"kind,omitzero" jsonschema:"calendars, task_lists or address_books; omit for all"`
}

type collectionOut struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind" jsonschema:"calendar, task_list or address_book"`
	Events    bool   `json:"events" jsonschema:"it holds events"`
	Tasks     bool   `json:"tasks" jsonschema:"it holds tasks"`
	ReadOnly  bool   `json:"read_only"`
	Default   bool   `json:"default" jsonschema:"where its account's new events, tasks or contacts go"`
}

type collectionsOut struct {
	Collections []collectionOut `json:"collections"`
	Notice      string          `json:"notice"`
}

var collectionKinds = map[api.CollectionKind]string{
	api.CollectionKindCalendar:    "calendar",
	api.CollectionKindTasklist:    "task_list",
	api.CollectionKindAddressbook: "address_book",
}

func (t *tools) listCollections(ctx context.Context, _ *sdk.CallToolRequest, in collectionsIn) (*sdk.CallToolResult, collectionsOut, error) {
	p := &api.AccountCollectionsParams{}
	switch in.Kind {
	case "":
	case "calendars":
		p.Kind = new(api.CollectionKindCalendar)
	case "task_lists":
		p.Kind = new(api.CollectionKindTasklist)
	case "address_books":
		p.Kind = new(api.CollectionKindAddressbook)
	default:
		return nil, collectionsOut{}, fmt.Errorf("kind must be calendars, task_lists or address_books")
	}
	cols, err := t.o.Client.Account().Collections(ctx, p)
	if err != nil {
		return nil, collectionsOut{}, err
	}
	out := collectionsOut{Collections: []collectionOut{}, Notice: Notice}
	for _, c := range cols {
		if c.Enabled {
			out.Collections = append(out.Collections, collectionOut{ID: c.ID, AccountID: c.AccountID, Name: c.Name,
				Kind: collectionKinds[c.Kind], Events: c.Events, Tasks: c.Tasks, ReadOnly: c.ReadOnly, Default: c.IsDefault})
		}
	}
	return nil, out, nil
}

// collectionNames maps the scope's collections of kind to their names.
func (t *tools) collectionNames(ctx context.Context, kind api.CollectionKind) (map[int64]string, error) {
	cols, err := t.o.Client.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(cols))
	for _, c := range cols {
		names[c.ID] = c.Name
	}
	return names, nil
}

type eventsIn struct {
	From       string `json:"from" jsonschema:"the first day, YYYY-MM-DD"`
	To         string `json:"to,omitzero" jsonschema:"the day after the last, YYYY-MM-DD, at most 31 days after from; default the day after from"`
	TimeZone   string `json:"time_zone,omitzero" jsonschema:"the IANA zone the days are in, such as Europe/Berlin; default maild's"`
	CalendarID int64  `json:"calendar_id,omitzero" jsonschema:"only this calendar, by its ID from list_collections"`
	Limit      int    `json:"limit,omitzero" jsonschema:"at most this many occurrences, the earliest, 1 to 200; default 100"`
}

type occurrenceOut struct {
	EventID      int64  `json:"event_id"`
	RecurrenceID string `json:"recurrence_id,omitzero" jsonschema:"which occurrence of a series, for read_event"`
	CalendarID   int64  `json:"calendar_id"`
	AccountID    int64  `json:"account_id"`
	Summary      string `json:"summary"`
	Location     string `json:"location,omitzero"`
	AllDay       bool   `json:"all_day"`
	Start        string `json:"start,omitzero" jsonschema:"a timed occurrence's start, RFC 3339 in the answer's zone"`
	End          string `json:"end,omitzero" jsonschema:"a timed occurrence's end, exclusive"`
	StartDate    string `json:"start_date,omitzero" jsonschema:"an all-day occurrence's first day"`
	EndDate      string `json:"end_date,omitzero" jsonschema:"an all-day occurrence's day after the last"`
	Status       string `json:"status" jsonschema:"confirmed, tentative or cancelled"`
	MyAnswer     string `json:"my_answer,omitzero" jsonschema:"the user's answer when invited: needsaction, accepted, declined, tentative or delegated"`
	Busy         bool   `json:"busy" jsonschema:"it counts as busy: not free time, not cancelled, not declined"`
}

type clashSide struct {
	EventID      int64  `json:"event_id"`
	RecurrenceID string `json:"recurrence_id,omitzero"`
	Summary      string `json:"summary"`
}

type clashOut struct {
	First  clashSide `json:"first"`
	Second clashSide `json:"second"`
	From   string    `json:"from" jsonschema:"RFC 3339: when the overlap starts"`
	To     string    `json:"to" jsonschema:"RFC 3339: when it ends"`
}

type eventsOut struct {
	TimeZone   string          `json:"time_zone" jsonschema:"the zone of the days and times; Local is maild's when it has no name"`
	Total      int             `json:"total" jsonschema:"how many occurrences the days hold"`
	Events     []occurrenceOut `json:"events"`
	Clashes    []clashOut      `json:"clashes" jsonschema:"pairs of busy timed occurrences among events that overlap; at most 50"`
	ClashCount int             `json:"clash_count" jsonschema:"how many such pairs there are"`
	Notice     string          `json:"notice"`
}

// busy reports whether an occurrence takes the user's time.
func busy(transparent bool, status api.EventStatus, answer *api.PartStat) bool {
	return !transparent && status != api.EventStatusCancelled && (answer == nil || *answer != api.PartStatDeclined)
}

func (t *tools) listEvents(ctx context.Context, _ *sdk.CallToolRequest, in eventsIn) (*sdk.CallToolResult, eventsOut, error) {
	if in.Limit == 0 {
		in.Limit = defaultEvents
	}
	if in.Limit < 1 || in.Limit > maxEvents {
		return nil, eventsOut{}, fmt.Errorf("limit must be between 1 and %d", maxEvents)
	}
	from, err := time.Parse(time.DateOnly, in.From)
	if err != nil {
		return nil, eventsOut{}, fmt.Errorf("from must be YYYY-MM-DD")
	}
	to := from.AddDate(0, 0, 1)
	if in.To != "" {
		if to, err = time.Parse(time.DateOnly, in.To); err != nil {
			return nil, eventsOut{}, fmt.Errorf("to must be YYYY-MM-DD")
		}
	}
	if !to.After(from) || to.After(from.AddDate(0, 0, maxEventDays)) {
		return nil, eventsOut{}, fmt.Errorf("to must be 1 to %d days after from", maxEventDays)
	}
	loc, zone, err := zoneOf(in.TimeZone)
	if err != nil {
		return nil, eventsOut{}, err
	}
	p := &api.CalendarRangeParams{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly)}
	if in.TimeZone != "" {
		p.TimeZone = &in.TimeZone
	}
	if in.CalendarID != 0 {
		p.CalendarIDs = []int64{in.CalendarID}
	}
	occ, err := t.o.Client.Calendar().Range(ctx, p)
	if err != nil {
		return nil, eventsOut{}, err
	}
	out := eventsOut{TimeZone: zone, Total: len(occ), Events: []occurrenceOut{}, Clashes: []clashOut{}, Notice: Notice}
	occ = occ[:min(len(occ), in.Limit)]
	for _, o := range occ {
		e := occurrenceOut{EventID: o.EventID, RecurrenceID: o.RecurrenceID, CalendarID: o.CalendarID, AccountID: o.AccountID,
			Summary: o.Summary, Location: o.Location, AllDay: o.AllDay, Status: string(o.Status),
			Busy: busy(o.Transparent, o.Status, o.Answer)}
		if o.Answer != nil {
			e.MyAnswer = string(*o.Answer)
		}
		if o.AllDay {
			e.StartDate, e.EndDate = o.StartDate, o.EndDate
		} else {
			e.Start, e.End = o.Start.In(loc).Format(time.RFC3339), o.End.In(loc).Format(time.RFC3339)
		}
		out.Events = append(out.Events, e)
	}
	for i, a := range occ {
		for j := i + 1; j < len(occ); j++ {
			b := occ[j]
			if a.AllDay || b.AllDay || !out.Events[i].Busy || !out.Events[j].Busy || !a.Start.Before(b.End) || !b.Start.Before(a.End) {
				continue
			}
			out.ClashCount++
			if len(out.Clashes) < maxClashes {
				start, end := a.Start, a.End
				if b.Start.After(start) {
					start = b.Start
				}
				if b.End.Before(end) {
					end = b.End
				}
				out.Clashes = append(out.Clashes, clashOut{
					First:  clashSide{EventID: a.EventID, RecurrenceID: a.RecurrenceID, Summary: a.Summary},
					Second: clashSide{EventID: b.EventID, RecurrenceID: b.RecurrenceID, Summary: b.Summary},
					From:   start.In(loc).Format(time.RFC3339), To: end.In(loc).Format(time.RFC3339)})
			}
		}
	}
	return nil, out, nil
}

type eventIn struct {
	EventID      int64  `json:"event_id" jsonschema:"the event, by its ID from list_events"`
	RecurrenceID string `json:"recurrence_id,omitzero" jsonschema:"which occurrence of a series, from list_events"`
}

type attendeeOut struct {
	Name   string `json:"name,omitzero"`
	Email  string `json:"email"`
	Role   string `json:"role,omitzero" jsonschema:"chair, required, optional or none"`
	Answer string `json:"answer,omitzero" jsonschema:"needsaction, accepted, declined, tentative or delegated"`
}

type eventOut struct {
	EventID       int64         `json:"event_id"`
	RecurrenceID  string        `json:"recurrence_id,omitzero"`
	CalendarID    int64         `json:"calendar_id"`
	Calendar      string        `json:"calendar" jsonschema:"the calendar's name"`
	AccountID     int64         `json:"account_id"`
	Summary       string        `json:"summary"`
	Location      string        `json:"location,omitzero"`
	Description   string        `json:"description"`
	Truncated     bool          `json:"truncated" jsonschema:"the description was cut at 32 KiB"`
	AllDay        bool          `json:"all_day"`
	Start         string        `json:"start,omitzero" jsonschema:"a timed event's start, RFC 3339 in its zone"`
	End           string        `json:"end,omitzero" jsonschema:"a timed event's end, exclusive"`
	StartDate     string        `json:"start_date,omitzero" jsonschema:"an all-day event's first day"`
	EndDate       string        `json:"end_date,omitzero" jsonschema:"an all-day event's day after the last"`
	TimeZone      string        `json:"time_zone,omitzero" jsonschema:"the IANA zone the event was made in"`
	Recurring     bool          `json:"recurring"`
	Recurrence    string        `json:"recurrence,omitzero" jsonschema:"a series' RRULE, RDATE and EXDATE lines as written"`
	Status        string        `json:"status"`
	Busy          bool          `json:"busy"`
	Organizer     *attendeeOut  `json:"organizer,omitzero"`
	Attendees     []attendeeOut `json:"attendees" jsonschema:"the first 100, without the organizer"`
	AttendeeCount int           `json:"attendee_count"`
	MyAnswer      string        `json:"my_answer,omitzero"`
	Notice        string        `json:"notice"`
}

func attendee(a api.Attendee) attendeeOut {
	return attendeeOut{Name: a.Name, Email: a.Email, Role: string(a.Role), Answer: string(a.Answer)}
}

func (t *tools) readEvent(ctx context.Context, _ *sdk.CallToolRequest, in eventIn) (*sdk.CallToolResult, eventOut, error) {
	p := &api.CalendarEventParams{ID: in.EventID}
	if in.RecurrenceID != "" {
		p.RecurrenceID = &in.RecurrenceID
	}
	e, err := t.o.Client.Calendar().Event(ctx, p)
	if err != nil {
		return nil, eventOut{}, err
	}
	names, err := t.collectionNames(ctx, api.CollectionKindCalendar)
	if err != nil {
		return nil, eventOut{}, err
	}
	out := eventOut{EventID: e.ID, RecurrenceID: e.RecurrenceID, CalendarID: e.CalendarID, Calendar: names[e.CalendarID],
		AccountID: e.AccountID, Summary: e.Summary, Location: e.Location, AllDay: e.AllDay, TimeZone: e.TimeZone,
		Recurring: e.Recurring, Recurrence: e.Recurrence, Status: string(e.Status), Busy: busy(e.Transparent, e.Status, e.Answer),
		Attendees: []attendeeOut{}, AttendeeCount: len(e.Attendees), Notice: Notice}
	out.Description, out.Truncated = cut(e.Description, maxText)
	if e.AllDay {
		out.StartDate, out.EndDate = e.StartDate, e.EndDate
	} else {
		loc := time.UTC
		if z, err := time.LoadLocation(e.TimeZone); err == nil && e.TimeZone != "" && e.TimeZone != "Local" {
			loc = z
		}
		out.Start, out.End = e.Start.In(loc).Format(time.RFC3339), e.End.In(loc).Format(time.RFC3339)
	}
	if e.Organizer != nil {
		o := attendee(*e.Organizer)
		out.Organizer = &o
	}
	for _, a := range e.Attendees[:min(len(e.Attendees), maxAttendees)] {
		out.Attendees = append(out.Attendees, attendee(a))
	}
	if e.Answer != nil {
		out.MyAnswer = string(*e.Answer)
	}
	return nil, out, nil
}

type tasksIn struct {
	Due       string `json:"due,omitzero" jsonschema:"today: due today or before; overdue: due before today; undated: with no due date; omit for every task"`
	ListID    int64  `json:"list_id,omitzero" jsonschema:"only this task list, by its ID from list_collections"`
	Completed bool   `json:"completed,omitzero" jsonschema:"include completed tasks; default false"`
	TimeZone  string `json:"time_zone,omitzero" jsonschema:"the IANA zone today is in; default maild's"`
	Limit     int    `json:"limit,omitzero" jsonschema:"at most this many tasks, 1 to 100; default 50"`
}

type taskOut struct {
	ID        int64  `json:"id"`
	ListID    int64  `json:"list_id"`
	List      string `json:"list" jsonschema:"the list's name"`
	AccountID int64  `json:"account_id"`
	ParentID  int64  `json:"parent_id,omitzero" jsonschema:"the task this one is a subtask of"`
	Title     string `json:"title"`
	Due       string `json:"due,omitzero" jsonschema:"YYYY-MM-DD"`
	Overdue   bool   `json:"overdue" jsonschema:"due before today and not completed"`
	Completed bool   `json:"completed"`
	HasNotes  bool   `json:"has_notes" jsonschema:"read_task reads them"`
}

type tasksOut struct {
	Today  string    `json:"today" jsonschema:"the day the answer took as today, YYYY-MM-DD"`
	Total  int       `json:"total" jsonschema:"how many tasks match"`
	Tasks  []taskOut `json:"tasks" jsonschema:"list by list, each parent followed by its subtasks"`
	Notice string    `json:"notice"`
}

func (t *tools) listTasks(ctx context.Context, _ *sdk.CallToolRequest, in tasksIn) (*sdk.CallToolResult, tasksOut, error) {
	if in.Limit == 0 {
		in.Limit = defaultTasks
	}
	if in.Limit < 1 || in.Limit > maxTasks {
		return nil, tasksOut{}, fmt.Errorf("limit must be between 1 and %d", maxTasks)
	}
	loc, _, err := zoneOf(in.TimeZone)
	if err != nil {
		return nil, tasksOut{}, err
	}
	now := t.now().In(loc)
	today := now.Format(time.DateOnly)
	p := &api.TasksListParams{}
	if in.ListID != 0 {
		p.ListID = &in.ListID
	}
	if in.Completed {
		p.Completed = &in.Completed
	}
	keep := func(api.Task) bool { return true }
	switch in.Due {
	case "":
	case "today":
		p.DueBefore = new(time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly))
		keep = func(t api.Task) bool { return t.Due != "" }
	case "overdue":
		p.DueBefore = &today
		keep = func(t api.Task) bool { return t.Due != "" }
	case "undated":
		keep = func(t api.Task) bool { return t.Due == "" }
	default:
		return nil, tasksOut{}, fmt.Errorf("due must be today, overdue or undated")
	}
	list, err := t.o.Client.Tasks().List(ctx, p)
	if err != nil {
		return nil, tasksOut{}, err
	}
	names, err := t.collectionNames(ctx, api.CollectionKindTasklist)
	if err != nil {
		return nil, tasksOut{}, err
	}
	out := tasksOut{Today: today, Tasks: []taskOut{}, Notice: Notice}
	for _, task := range list {
		if !keep(task) {
			continue
		}
		out.Total++
		if len(out.Tasks) == in.Limit {
			continue
		}
		o := taskOut{ID: task.ID, ListID: task.ListID, List: names[task.ListID], AccountID: task.AccountID, Title: task.Title,
			Due: task.Due, Overdue: task.Due != "" && task.Due < today && !task.Completed, Completed: task.Completed,
			HasNotes: strings.TrimSpace(task.Notes) != ""}
		if task.ParentID != nil {
			o.ParentID = *task.ParentID
		}
		out.Tasks = append(out.Tasks, o)
	}
	return nil, out, nil
}

type taskIn struct {
	TaskID int64 `json:"task_id" jsonschema:"the task, by its ID from list_tasks"`
}

type taskFullOut struct {
	ID          int64  `json:"id"`
	ListID      int64  `json:"list_id"`
	List        string `json:"list"`
	AccountID   int64  `json:"account_id"`
	ParentID    int64  `json:"parent_id,omitzero"`
	Title       string `json:"title"`
	Notes       string `json:"notes"`
	Truncated   bool   `json:"truncated" jsonschema:"the notes were cut at 32 KiB"`
	Due         string `json:"due,omitzero"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at,omitzero" jsonschema:"RFC 3339"`
	MessageID   int64  `json:"message_id,omitzero" jsonschema:"the message the task was made from, for read_thread"`
	// The task's message's link, as other tools answer a message's.
	MessageIDHeader string `json:"message_id_header,omitzero" jsonschema:"that message's Message-ID header, without angle brackets; absent when it has none"`
	MessageURL      string `json:"message_url,omitzero" jsonschema:"that message's permanent lodge URL, to link it or for lodge.open; use it as it is"`
	Notice          string `json:"notice"`
}

func (t *tools) readTask(ctx context.Context, _ *sdk.CallToolRequest, in taskIn) (*sdk.CallToolResult, taskFullOut, error) {
	task, err := t.o.Client.Tasks().Get(ctx, &api.TasksGetParams{ID: in.TaskID})
	if err != nil {
		return nil, taskFullOut{}, err
	}
	names, err := t.collectionNames(ctx, api.CollectionKindTasklist)
	if err != nil {
		return nil, taskFullOut{}, err
	}
	out := taskFullOut{ID: task.ID, ListID: task.ListID, List: names[task.ListID], AccountID: task.AccountID, Title: task.Title,
		Due: task.Due, Completed: task.Completed, Notice: Notice}
	out.Notes, out.Truncated = cut(task.Notes, maxText)
	if task.ParentID != nil {
		out.ParentID = *task.ParentID
	}
	if task.CompletedAt != nil {
		out.CompletedAt = task.CompletedAt.UTC().Format(time.RFC3339)
	}
	if task.MessageID != nil {
		out.MessageID = *task.MessageID
		headers, err := t.messageIDHeaders(ctx, []int64{out.MessageID})
		if err != nil {
			return nil, taskFullOut{}, err
		}
		out.MessageIDHeader, out.MessageURL = messageLink(headers[out.MessageID])
	}
	return nil, out, nil
}

type contactsIn struct {
	Query string `json:"query" jsonschema:"words that start the person's name, organization or email address; 1 to 256 bytes"`
	Limit int    `json:"limit,omitzero" jsonschema:"at most this many people, 1 to 50; default 20"`
}

type personOut struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Organization string `json:"organization,omitzero"`
	Email        string `json:"email,omitzero" jsonschema:"their first email address"`
}

type contactsOut struct {
	Total  int         `json:"total" jsonschema:"how many people match"`
	People []personOut `json:"people" jsonschema:"by name"`
	Notice string      `json:"notice"`
}

func (t *tools) searchContacts(ctx context.Context, _ *sdk.CallToolRequest, in contactsIn) (*sdk.CallToolResult, contactsOut, error) {
	if in.Limit == 0 {
		in.Limit = defaultPeople
	}
	if in.Limit < 1 || in.Limit > maxPeople {
		return nil, contactsOut{}, fmt.Errorf("limit must be between 1 and %d", maxPeople)
	}
	if strings.TrimSpace(in.Query) == "" || len(in.Query) > maxContactQuery {
		return nil, contactsOut{}, fmt.Errorf("query must be 1 to %d bytes", maxContactQuery)
	}
	people, err := t.o.Client.People().List(ctx, &api.PeopleListParams{Query: &in.Query})
	if err != nil {
		return nil, contactsOut{}, err
	}
	out := contactsOut{Total: len(people), People: []personOut{}, Notice: Notice}
	for _, p := range people[:min(len(people), in.Limit)] {
		out.People = append(out.People, personOut{ID: p.ID, Name: p.DisplayName, Organization: p.Organization, Email: p.Email})
	}
	return nil, out, nil
}

type contactIn struct {
	PersonID int64 `json:"person_id" jsonschema:"the person, by their ID from search_contacts"`
}

type labeledOut struct {
	Label string `json:"label,omitzero" jsonschema:"home, work, mobile, other, or the card's own"`
	Value string `json:"value"`
}

type postalOut struct {
	Label    string `json:"label,omitzero"`
	Street   string `json:"street,omitzero"`
	Locality string `json:"locality,omitzero"`
	Region   string `json:"region,omitzero"`
	Postcode string `json:"postcode,omitzero"`
	Country  string `json:"country,omitzero"`
}

type cardOut struct {
	ContactID     int64        `json:"contact_id"`
	AddressBookID int64        `json:"address_book_id"`
	AccountID     int64        `json:"account_id"`
	Name          string       `json:"name"`
	GivenName     string       `json:"given_name,omitzero"`
	FamilyName    string       `json:"family_name,omitzero"`
	Nickname      string       `json:"nickname,omitzero"`
	Organization  string       `json:"organization,omitzero"`
	Title         string       `json:"title,omitzero"`
	Emails        []labeledOut `json:"emails"`
	Phones        []labeledOut `json:"phones"`
	Addresses     []postalOut  `json:"addresses"`
	URLs          []labeledOut `json:"urls"`
	Birthday      string       `json:"birthday,omitzero" jsonschema:"YYYY-MM-DD, or --MM-DD without a year"`
	Note          string       `json:"note,omitzero"`
	Truncated     bool         `json:"truncated" jsonschema:"the note was cut at 32 KiB"`
	ReadOnly      bool         `json:"read_only"`
}

type contactOut struct {
	PersonID     int64     `json:"person_id"`
	Name         string    `json:"name"`
	Organization string    `json:"organization,omitzero"`
	Contacts     []cardOut `json:"contacts"`
	Notice       string    `json:"notice"`
}

func labeled(list []api.LabeledValue) []labeledOut {
	out := make([]labeledOut, 0, len(list))
	for _, v := range list {
		out = append(out, labeledOut{Label: v.Label, Value: v.Value})
	}
	return out
}

func (t *tools) readContact(ctx context.Context, _ *sdk.CallToolRequest, in contactIn) (*sdk.CallToolResult, contactOut, error) {
	p, err := t.o.Client.People().Get(ctx, &api.PeopleGetParams{ID: in.PersonID})
	if err != nil {
		return nil, contactOut{}, err
	}
	out := contactOut{PersonID: p.ID, Name: p.DisplayName, Organization: p.Organization, Contacts: []cardOut{}, Notice: Notice}
	for _, c := range p.Contacts {
		card := cardOut{ContactID: c.ID, AddressBookID: c.CollectionID, AccountID: c.AccountID, Name: c.DisplayName,
			GivenName: c.GivenName, FamilyName: c.FamilyName, Nickname: c.Nickname, Organization: c.Organization, Title: c.Title,
			Emails: labeled(c.Emails), Phones: labeled(c.Phones), Addresses: []postalOut{}, URLs: labeled(c.URLs),
			Birthday: c.Birthday, ReadOnly: c.ReadOnly}
		card.Note, card.Truncated = cut(c.Note, maxText)
		for _, a := range c.Addresses {
			card.Addresses = append(card.Addresses, postalOut{Label: a.Label, Street: a.Street, Locality: a.Locality,
				Region: a.Region, Postcode: a.Postcode, Country: a.Country})
		}
		out.Contacts = append(out.Contacts, card)
	}
	return nil, out, nil
}
