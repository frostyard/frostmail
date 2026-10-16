package pimsync_test

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davsession"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// Graph's writes (plan 0010, Phase 3b): the app's edits reach Outlook as
// POST, PATCH and DELETE with only the fields they changed.

type graphWriteEnv struct {
	*graphEnv
	cal      api.CalendarService
	contacts api.PeopleService
	events   []api.EventEnvelope
}

func newGraphWriteEnv(t *testing.T) *graphWriteEnv {
	t.Helper()
	e := &graphWriteEnv{graphEnv: newGraphEnv(t, allGraphScopes)}
	e.srv.User = "cy@hotmail.com"
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	e.m = pimsync.New(e.db, secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json")),
		slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{
			GraphURL: e.srv.URL, Tokens: graphTokens{graph: "graph-token"}, Now: func() time.Time { return graphNow },
			Local: newYork,
		})
	e.db.OnCommit = func(evs []api.EventEnvelope) { e.events = append(e.events, evs...) }
	eng := engine.New(engine.Deps{DB: e.db, PIM: e.m})
	e.cal, e.contacts = eng.Calendar(), eng.People()
	return e
}

// event is the ID of the event (single or master) with the summary.
func (e *graphWriteEnv) event(summary string) int64 {
	e.t.Helper()
	rows, err := e.db.Events(e.t.Context(), store.EventsFilter{})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, r := range rows {
		if r.Summary == summary && r.RecurrenceID == "" {
			return r.ID
		}
	}
	e.t.Fatalf("no event %q in %+v", summary, rows)
	return 0
}

func (e *graphWriteEnv) object(eventID int64) store.Object {
	e.t.Helper()
	row, err := e.db.Event(e.t.Context(), eventID)
	if err != nil {
		e.t.Fatal(err)
	}
	obj, err := e.db.GetObject(e.t.Context(), row.ObjectID)
	if err != nil {
		e.t.Fatal(err)
	}
	return obj
}

func (e *graphWriteEnv) etag(id string) string {
	e.t.Helper()
	doc := e.srv.Event(id)
	if doc == nil {
		e.t.Fatalf("Graph has no event %s", id)
	}
	return doc["@odata.etag"].(string)
}

func (e *graphWriteEnv) refused(name string) []string {
	var out []string
	for _, ev := range e.events {
		if ev.Event != name {
			continue
		}
		var r struct {
			Reason  string `json:"reason"`
			Summary string `json:"summary"`
			Name    string `json:"name"`
		}
		if err := json.Unmarshal(ev.Data, &r); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, r.Reason+" "+r.Summary+r.Name)
	}
	return out
}

func TestGraphEventCreateEditDelete(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.pass()
	start := utcAt("2026-10-27T14:00Z")
	if _, err := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: api.EventFields{Summary: ptr2("FM review"),
		AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("America/New_York"),
		Alarms: []int64{15}}}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) != 1 || !strings.HasPrefix(writes[0], "POST me/calendars/cal-1/events ") ||
		!strings.Contains(writes[0], `"start":{"dateTime":"2026-10-27T10:00:00","timeZone":"Eastern Standard Time"}`) ||
		!strings.Contains(writes[0], `"subject":"FM review"`) || !strings.Contains(writes[0], `"reminderMinutesBeforeStart":15`) ||
		!strings.Contains(writes[0], `"transactionId":"`) {
		t.Fatalf("writes = %q", writes)
	}
	// The event is Outlook's now: its ID, its UID, its JSON.
	id := e.event("FM review")
	obj := e.object(id)
	graphID := e.srv.Events("cal-1")[0]
	if obj.Href != graphID || obj.ETag != e.etag(graphID) || !strings.Contains(string(obj.Remote), `"id":"`+graphID+`"`) ||
		!strings.Contains(string(obj.Raw), "UID:040000008200E00074C5B7101A82E008-") {
		t.Errorf("the created object = %+v", obj)
	}
	if got := e.occurrences(); !slices.Equal(got, []string{"10-27T14:00 FM review"}) {
		t.Errorf("occurrences = %v", got)
	}

	etag := obj.ETag
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Location: ptr2("Room 4")}}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	want := []string{"PATCH me/events/" + graphID + " [" + etag + `] {"location":{"displayName":"Room 4"}}`}
	if got := e.srv.WritesSince(n); !slices.Equal(got, want) {
		t.Errorf("edit writes = %q, want %q", got, want)
	}

	id = e.event("FM review")
	etag = e.object(id).ETag
	if _, err := e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: id}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	if got, want := e.srv.WritesSince(n), []string{"DELETE me/events/" + graphID + " [" + etag + "]"}; !slices.Equal(got, want) {
		t.Errorf("delete writes = %q, want %q", got, want)
	}
	if got := e.srv.Events("cal-1"); len(got) != 0 {
		t.Errorf("Graph still has %v", got)
	}
}

// An edit leaves what Frostmail does not model as Outlook has it: the
// HTML body, the categories, the sensitivity.
func TestGraphEditKeepsWhatItDoesNotTouch(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", strings.Replace(graphSingle, `"subject": "Review",`, `"subject": "Review", "categories": ["Work"],
		"sensitivity": "private", "body": {"contentType": "html", "content": "<p>Bring the <b>notes</b>.</p>"},`, 1))
	e.pass()
	start := utcAt("2026-10-27T15:00Z")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Review"), Fields: api.EventFields{Summary: ptr2("Review, moved"),
		AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("America/New_York")}}); err != nil {
		t.Fatal(err)
	}
	etag := e.etag("AAMk-single")
	n := e.srv.WriteCount()
	e.pass()
	want := []string{"PATCH me/events/AAMk-single [" + etag + `] {"end":{"dateTime":"2026-10-27T12:00:00","timeZone":"Eastern Standard Time"},` +
		`"start":{"dateTime":"2026-10-27T11:00:00","timeZone":"Eastern Standard Time"},"subject":"Review, moved"}`}
	if got := e.srv.WritesSince(n); !slices.Equal(got, want) {
		t.Errorf("writes = %q\nwant %q", got, want)
	}
	doc := e.srv.Event("AAMk-single")
	if body, _ := doc["body"].(map[string]any); body["contentType"] != "html" || doc["sensitivity"] != "private" {
		t.Errorf("Graph's event = %v", doc)
	}
	if got := e.occurrences(); !slices.Equal(got, []string{"10-27T15:00 Review, moved"}) {
		t.Errorf("occurrences = %v", got)
	}
}

// Another client changed another field: the edit goes again on Graph's
// version; the same field: Graph's version stands.
func TestGraphEditAfterAnotherClient(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSingle)
	e.pass()
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Review"), Fields: api.EventFields{Location: ptr2("Room 4")}}); err != nil {
		t.Fatal(err)
	}
	e.srv.PutEvent("cal-1", strings.Replace(graphSingle, `"subject": "Review"`, `"subject": "Review", "body": {"contentType": "text", "content": "Agenda in the doc"}`, 1))
	stale := e.object(e.event("Review")).ETag
	n := e.srv.WriteCount()
	e.pass()
	fresh := e.etag("AAMk-single")
	writes := e.srv.WritesSince(n)
	if len(writes) != 2 || !strings.HasPrefix(writes[0], "PATCH me/events/AAMk-single ["+stale+"]") ||
		writes[1] != "PATCH me/events/AAMk-single [W/\"ck-"+strings.TrimSuffix(strings.TrimPrefix(fresh, `W/"ck-`), `"`)+"\"] {\"location\":{\"displayName\":\"Room 4\"}}" &&
			!strings.Contains(writes[1], `{"location":{"displayName":"Room 4"}}`) {
		t.Errorf("writes = %q", writes)
	}
	doc := e.srv.Event("AAMk-single")
	if loc, _ := doc["location"].(map[string]any); loc["displayName"] != "Room 4" || doc["body"].(map[string]any)["content"] != "Agenda in the doc" {
		t.Errorf("Graph's event = %v", doc)
	}
	if r := e.refused("calendar.refused"); len(r) != 0 {
		t.Errorf("refused = %v", r)
	}

	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Review"), Fields: api.EventFields{Summary: ptr2("Review Q4")}}); err != nil {
		t.Fatal(err)
	}
	e.srv.PutEvent("cal-1", strings.Replace(graphSingle, `"subject": "Review"`, `"subject": "Roadmap"`, 1))
	e.pass()
	if r := e.refused("calendar.refused"); !slices.Equal(r, []string{"changed Review Q4"}) {
		t.Errorf("refused = %v", r)
	}
	if got := e.occurrences(); !slices.Equal(got, []string{"10-27T14:00 Roadmap"}) {
		t.Errorf("occurrences = %v", got)
	}
}

func TestGraphSeriesOccurrences(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSeries("Standup"))
	e.pass()
	id := e.event("Standup")
	oct29, oct27, oct30 := "2026-10-29T13:30:00.000Z", "2026-10-27T13:30:00.000Z", "2026-10-30T13:30:00.000Z"

	// One occurrence moves to a room; the exception gets a name; another
	// occurrence goes.
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: &oct29, Scope: scope(api.EventScopeThis),
		Fields: api.EventFields{Location: ptr2("Room 2")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: &oct27, Scope: scope(api.EventScopeThis),
		Fields: api.EventFields{Summary: ptr2("Standup, later")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: id, RecurrenceID: &oct30, Scope: scope(api.EventScopeThis)}); err != nil {
		t.Fatal(err)
	}
	etag := e.etag("AAMk-master")
	n := e.srv.WriteCount()
	e.pass()
	want := []string{
		`PATCH me/events/AAMk-exception {"subject":"Standup, later"}`,
		`PATCH me/events/AAMk-master-occ-` + oct29 + ` {"location":{"displayName":"Room 2"}}`,
		`DELETE me/events/AAMk-master-occ-` + oct30,
	}
	if got := e.srv.WritesSince(n); !slices.Equal(got, want) {
		t.Errorf("writes = %q\nwant %q", got, want)
	}
	if e.etag("AAMk-master") == etag {
		t.Error("Graph's series did not change")
	}
	wantOcc := []string{"10-26T13:30 Standup", "10-27T14:00 Standup, later", "10-29T13:30 Standup"}
	if got := e.occurrences(); !slices.Equal(got, wantOcc) {
		t.Errorf("occurrences = %v, want %v", got, wantOcc)
	}
	if obj := e.object(e.event("Standup")); !strings.Contains(string(obj.Raw), "LOCATION:Room 2") ||
		!strings.Contains(string(obj.Remote), `"Room 2"`) {
		t.Errorf("the series here = %s", obj.Raw)
	}

	// The whole series is renamed: the master, and the exceptions.
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Standup"), RecurrenceID: &oct29, Scope: scope(api.EventScopeAll),
		Fields: api.EventFields{Summary: ptr2("Daily standup")}}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) == 0 || !strings.HasPrefix(writes[0], "PATCH me/events/AAMk-master [") || !strings.HasSuffix(writes[0], `{"subject":"Daily standup"}`) {
		t.Errorf("writes = %q", writes)
	}
	for _, o := range e.occurrences() {
		if !strings.HasSuffix(o, " Daily standup") && !strings.HasSuffix(o, " Standup, later") {
			t.Errorf("after the rename: %v", e.occurrences())
			break
		}
	}
}

// This and Future ends the series before the occurrence and starts a new
// one there. Outlook keeps the series' exception and cancellation before
// then, as only its end changed.
func TestGraphSeriesSplit(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSeries("Standup"))
	e.pass()
	oct29 := "2026-10-29T13:30:00.000Z"
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Standup"), RecurrenceID: &oct29, Scope: scope(api.EventScopeFuture),
		Fields: api.EventFields{Summary: ptr2("Standup v2")}}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) != 2 || !strings.HasPrefix(writes[0], "PATCH me/events/AAMk-master [") ||
		!strings.Contains(writes[0], `"range":{"endDate":"2026-10-28","recurrenceTimeZone":"Eastern Standard Time","startDate":"2026-10-26","type":"endDate"}`) ||
		!strings.HasPrefix(writes[1], "POST me/calendars/cal-1/events ") || !strings.Contains(writes[1], `"subject":"Standup v2"`) ||
		!strings.Contains(writes[1], `"range":{"numberOfOccurrences":2,"recurrenceTimeZone":"Eastern Standard Time","startDate":"2026-10-29","type":"numbered"}`) {
		t.Errorf("writes = %q", writes)
	}
	want := []string{"10-26T13:30 Standup", "10-27T14:00 Standup", "10-29T13:30 Standup v2", "10-30T13:30 Standup v2"}
	if got := e.occurrences(); !slices.Equal(got, want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
}

// A series moved to another time loses its exceptions and cancellations
// in Outlook: they are written again, on the series as it is then.
func TestGraphSeriesMoveRewritesItsExceptions(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSeries("Standup"))
	e.pass()
	first := "2026-10-26T13:30:00.000Z"
	start := utcAt("2026-10-26T13:45Z")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Standup"), RecurrenceID: &first, Scope: scope(api.EventScopeAll),
		Fields: api.EventFields{AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(15 * time.Minute)), TimeZone: ptr2("America/New_York")}}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	var moved, rewritten, cancelled bool
	for _, w := range writes {
		moved = moved || strings.HasPrefix(w, "PATCH me/events/AAMk-master [") && strings.Contains(w, `"start":{"dateTime":"2026-10-26T09:45:00"`)
		rewritten = rewritten || strings.HasPrefix(w, "PATCH me/events/AAMk-master-occ-2026-10-27T13:45:00.000Z ")
		cancelled = cancelled || w == "DELETE me/events/AAMk-master-occ-2026-10-28T13:45:00.000Z"
	}
	if !moved || !rewritten || !cancelled {
		t.Errorf("writes = %q", writes)
	}
	want := []string{"10-26T13:45 Standup", "10-27T14:00 Standup", "10-29T13:45 Standup", "10-30T13:45 Standup"}
	if got := e.occurrences(); !slices.Equal(got, want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
}

const graphInvitation = `{"id": "AAMk-inv", "iCalUId": "inv-uid", "type": "singleInstance", "subject": "FM planning",
	"start": {"dateTime": "2026-11-03T15:00:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-11-03T16:00:00.0000000", "timeZone": "UTC"},
	"originalStartTimeZone": "Pacific Standard Time", "originalEndTimeZone": "Pacific Standard Time", "isOrganizer": false,
	"organizer": {"emailAddress": {"name": "Bee", "address": "bee@example.com"}},
	"attendees": [{"type": "required", "status": {"response": "none"}, "emailAddress": {"name": "Cy", "address": "cy@hotmail.com"}}],
	"responseStatus": {"response": "notResponded"}}`

// An answer goes to Graph, which tells the organizer: no reply is mailed.
func TestGraphAnswersAnInvitation(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphInvitation)
	e.pass()
	id := e.event("FM planning")
	if _, err := e.cal.Respond(ctx, &api.CalendarRespondParams{EventID: &id, Answer: api.PartStatAccepted}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	etag := e.etag("AAMk-inv")
	if got, want := e.srv.WritesSince(n), []string{`POST me/events/AAMk-inv/accept {"sendResponse":true}`}; !slices.Equal(got, want) {
		t.Errorf("writes = %q, want %q", got, want)
	}
	if r := e.object(e.event("FM planning")); r.ETag != etag {
		t.Errorf("the answered copy's etag %q, Graph's %q", r.ETag, etag)
	}
	row, err := e.db.Event(ctx, e.event("FM planning"))
	if err != nil || row.PartStat != "accepted" {
		t.Errorf("the answer here = %+v, %v", row, err)
	}
	if outbox, err := e.db.ListOutbox(ctx, e.acct.ID); err != nil || len(outbox) != 0 {
		t.Errorf("outbox = %+v, %v", outbox, err)
	}
}

// Outlook takes a declined invitation off the calendar: so does the
// replay, at once.
func TestGraphDeclineRemovesTheInvitation(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphInvitation)
	e.pass()
	id := e.event("FM planning")
	if _, err := e.cal.Respond(ctx, &api.CalendarRespondParams{EventID: &id, Answer: api.PartStatDeclined}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	if got, want := e.srv.WritesSince(n), []string{`POST me/events/AAMk-inv/decline {"sendResponse":true}`}; !slices.Equal(got, want) {
		t.Errorf("writes = %q, want %q", got, want)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now().Add(time.Hour)); len(ops) != 0 {
		t.Errorf("ops left = %+v", ops)
	}
	if got := e.occurrences(); len(got) != 0 {
		t.Errorf("occurrences = %v", got)
	}
}

// The organizer's own meeting: invitees go to Graph, which invites them.
func TestGraphOrganizerInvites(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.pass()
	start := utcAt("2026-11-04T15:00Z")
	if _, err := e.cal.Create(ctx, &api.CalendarCreateParams{Notify: ptr2(true), Fields: api.EventFields{Summary: ptr2("FM sync"),
		AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("UTC"),
		Attendees: []api.Invitee{{Email: "bee@example.com"}}}}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) != 1 || !strings.Contains(writes[0], `"attendees":[{"emailAddress":{"address":"bee@example.com","name":""},"type":"required"}]`) {
		t.Errorf("writes = %q", writes)
	}
}

func TestGraphContactCreateEditPhotoDelete(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.pass()
	w, err := e.contacts.Create(ctx, &api.PeopleCreateParams{Fields: api.ContactFields{GivenName: ptr2("FM"), FamilyName: ptr2("Kai"),
		Emails: []api.LabeledValue{{Label: "home", Value: "fm.kai@example.com"}}, Phones: []api.LabeledValue{{Label: "mobile", Value: "+1 555 0100"}}}})
	if err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) != 1 || !strings.HasPrefix(writes[0], "POST me/contacts ") || !strings.Contains(writes[0], `"displayName":"FM Kai"`) ||
		!strings.Contains(writes[0], `"emailAddresses":[{"address":"fm.kai@example.com"}]`) || !strings.Contains(writes[0], `"mobilePhone":"+1 555 0100"`) {
		t.Fatalf("writes = %q", writes)
	}
	people, err := e.db.People(ctx, "")
	if err != nil || len(people) != 1 {
		t.Fatalf("people = %+v, %v", people, err)
	}
	contact := people[0].ContactIDs[0]
	obj, err := e.db.GetObject(ctx, contact)
	if err != nil || !strings.HasPrefix(obj.Href, "AAMk-c") {
		t.Fatalf("the created contact = %+v, %v (created as %d)", obj, err, w.ContactID)
	}

	if _, err := e.contacts.Update(ctx, &api.PeopleUpdateParams{ID: contact, Fields: api.ContactFields{Note: ptr2("Likes tea")}}); err != nil {
		t.Fatal(err)
	}
	photo := []byte("\xff\xd8\xff\xe0 a jpeg")
	if _, err := e.contacts.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: contact,
		Photo: &api.Photo{ContentType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(photo)}}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	want := []string{"PATCH me/contacts/" + obj.Href + " [" + obj.ETag + `] {"personalNotes":"Likes tea"}`,
		"PUT me/contacts/" + obj.Href + "/photo/$value " + strconv.Itoa(len(photo)) + " bytes of image/jpeg"}
	if got := e.srv.WritesSince(n); !slices.Equal(got, want) {
		t.Errorf("writes = %q\nwant %q", got, want)
	}
	if doc, got := e.srv.Contact(obj.Href); string(got) != string(photo) || doc["personalNotes"] != "Likes tea" {
		t.Errorf("Graph's contact = %v, photo %q", doc, got)
	}
	if got := e.people(); !slices.Equal(got, []string{"FM Kai fm.kai@example.com (photo)"}) {
		t.Errorf("people = %v", got)
	}

	// Graph cannot delete a photo: the rest of the change is written, and
	// the photo stays, refused.
	if _, err := e.contacts.Update(ctx, &api.PeopleUpdateParams{ID: contact, Fields: api.ContactFields{Title: ptr2("Writer")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.contacts.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: contact}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	if got := e.srv.WritesSince(n); len(got) != 1 || !strings.HasSuffix(got[0], `{"jobTitle":"Writer"}`) {
		t.Errorf("writes = %q", got)
	}
	if r := e.refused("people.refused"); !slices.Equal(r, []string{"refused FM Kai"}) {
		t.Errorf("refused = %v", r)
	}
	if got := e.people(); !slices.Equal(got, []string{"FM Kai fm.kai@example.com (photo)"}) {
		t.Errorf("after the refusal: people = %v", got)
	}

	obj, _ = e.db.GetObject(ctx, contact)
	if _, err := e.contacts.Delete(ctx, &api.PeopleDeleteParams{ID: contact}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	if got, want := e.srv.WritesSince(n), []string{"DELETE me/contacts/" + obj.Href + " [" + obj.ETag + "]"}; !slices.Equal(got, want) {
		t.Errorf("writes = %q, want %q", got, want)
	}
	if got := e.people(); len(got) != 0 {
		t.Errorf("people = %v", got)
	}
}

// What Outlook cannot store is refused here and never sent.
func TestGraphRefusesWhatOutlookCannotStore(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutContact("contacts", graphContact, nil)
	e.pass()
	people, _ := e.db.People(ctx, "")
	if _, err := e.contacts.Update(ctx, &api.PeopleUpdateParams{ID: people[0].ContactIDs[0], Fields: api.ContactFields{Birthday: ptr2("--05-17")}}); err != nil {
		t.Fatal(err)
	}
	start := utcAt("2026-10-30T14:00Z")
	if _, err := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: api.EventFields{Summary: ptr2("FM last day"),
		AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("America/New_York"),
		Rrule: ptr2("FREQ=MONTHLY;BYMONTHDAY=-1")}}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	if got := e.srv.WritesSince(n); len(got) != 0 {
		t.Errorf("writes = %q", got)
	}
	if r := e.refused("people.refused"); !slices.Equal(r, []string{"refused Dee Example"}) {
		t.Errorf("people refused = %v", r)
	}
	if r := e.refused("calendar.refused"); !slices.Equal(r, []string{"refused FM last day"}) {
		t.Errorf("calendar refused = %v", r)
	}
	if got := e.occurrences(); len(got) != 0 {
		t.Errorf("the refused series is still here: %v", got)
	}
}

func TestGraphCollectionsWritableAsOutlookSays(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	e.srv.AddCalendar("cal-holidays", "US Holidays", false)
	e.pass()
	cols, err := e.db.Collections(t.Context(), store.CollectionFilter{AccountID: e.acct.ID})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cols {
		got = append(got, string(c.Kind)+" "+c.Name+map[bool]string{true: " read-only", false: ""}[c.ReadOnly])
	}
	slices.Sort(got)
	if want := []string{"addressbook Contacts", "calendar Calendar", "calendar US Holidays read-only"}; !slices.Equal(got, want) {
		t.Errorf("collections = %v, want %v", got, want)
	}
}

// Undoing This Event writes the exception back to the occurrence the
// series makes; undoing an occurrence's delete is refused, as Graph cannot
// bring a cancelled occurrence back.
func TestGraphUndoInASeries(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", strings.Replace(graphSeries("Standup"), `"cancelledOccurrences": ["OID.AAMk-master.2026-10-28"],`, "", 1))
	e.pass()
	oct29, oct30 := "2026-10-29T13:30:00.000Z", "2026-10-30T13:30:00.000Z"
	start := utcAt("2026-10-29T14:00Z")
	w, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Standup"), RecurrenceID: &oct29, Scope: scope(api.EventScopeThis),
		Fields: api.EventFields{AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(15 * time.Minute)), TimeZone: ptr2("America/New_York")}})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: w.RestoreID}); err != nil {
		t.Fatal(err)
	}
	n := e.srv.WriteCount()
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) != 1 || !strings.HasPrefix(writes[0], "PATCH me/events/AAMk-x") ||
		!strings.HasSuffix(writes[0], `{"end":{"dateTime":"2026-10-29T09:45:00","timeZone":"Eastern Standard Time"},"start":{"dateTime":"2026-10-29T09:30:00","timeZone":"Eastern Standard Time"}}`) {
		t.Errorf("the undo's writes = %q", writes)
	}
	want := []string{"10-26T13:30 Standup", "10-27T14:00 Standup", "10-28T13:30 Standup", "10-29T13:30 Standup", "10-30T13:30 Standup"}
	if got := e.occurrences(); !slices.Equal(got, want) {
		t.Errorf("after the undo: occurrences = %v", got)
	}

	gone, err := e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: e.event("Standup"), RecurrenceID: &oct30, Scope: scope(api.EventScopeThis)})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: gone.RestoreID}); err != nil {
		t.Fatal(err)
	}
	n = e.srv.WriteCount()
	e.pass()
	if got := e.srv.WritesSince(n); len(got) != 0 {
		t.Errorf("the refused undo wrote %q", got)
	}
	if r := e.refused("calendar.refused"); !slices.Equal(r, []string{"refused Standup"}) {
		t.Errorf("refused = %v", r)
	}
	if got := e.occurrences(); slices.Contains(got, "10-30T13:30 Standup") {
		t.Errorf("the deleted occurrence is back here: %v", got)
	}
}

// The recorded Microsoft session (internal/davsession.Microsoft) runs on
// the fake as on Outlook, refusing nothing and leaving nothing behind.
func TestMicrosoftSessionOnTheFake(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutContact("contacts", graphContact, nil)
	var steps, refused []string
	err := davsession.Microsoft(ctx, davsession.Session{
		Calendar: e.cal,
		People:   e.contacts,
		Pass: func(ctx context.Context) error {
			n := len(e.events)
			err := e.m.Pass(ctx, e.acct.ID)
			for _, ev := range e.events[n:] {
				if strings.HasSuffix(ev.Event, ".refused") {
					refused = append(refused, steps[len(steps)-1]+": "+string(ev.Data))
				}
			}
			return err
		},
		Step: func(name string) { steps = append(steps, name) },
	})
	if err != nil {
		t.Fatalf("after %q: %v", steps, err)
	}
	if len(refused) != 0 {
		t.Errorf("refused: %q", refused)
	}
	if got := e.srv.Events("cal-1"); len(got) != 0 {
		t.Errorf("the session left %v in Graph", got)
	}
	if got := e.people(); !slices.Equal(got, []string{"Dee Example dee@example.com"}) {
		t.Errorf("people = %v", got)
	}
}
