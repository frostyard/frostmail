package pimsync_test

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// An edit made while the object's put is being written is not lost: the
// change goes on from the version the put left, and the next pass writes
// the rest (found in the Phase 3b write session, 2026-10-16).

func TestEditDuringAPutReachesTheServer(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	id := e.event("Planning")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Summary: ptr2("Planning Q4")}}); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	e.dav.FailWith(func(r *http.Request) int {
		if r.Method == http.MethodPut {
			once.Do(func() {
				if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Location: ptr2("Room 4")}}); err != nil {
					t.Error(err)
				}
			})
		}
		return 0
	})
	e.pass()
	e.dav.FailWith(nil)
	e.pass()
	hasLines(t, e.server(e.work+"planning.ics"), "SUMMARY:Planning Q4", "LOCATION:Room 4")
	if r := e.refused(); len(r) != 0 {
		t.Errorf("refused = %+v", r)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 0 {
		t.Errorf("ops left = %+v", ops)
	}
}

func TestGraphEditDuringAWrite(t *testing.T) {
	e := newGraphWriteEnv(t)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSingle)
	e.pass()
	id := e.event("Review")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Location: ptr2("Room 4")}}); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	e.srv.OnWrite(func(string, string) {
		once.Do(func() {
			if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Summary: ptr2("Review Q4")}}); err != nil {
				t.Error(err)
			}
		})
	})
	n := e.srv.WriteCount()
	e.pass()
	e.srv.OnWrite(nil)
	if got := e.occurrences(); !slices.Equal(got, []string{"10-27T14:00 Review Q4"}) {
		t.Errorf("between the passes: occurrences = %v", got)
	}
	e.pass()
	writes := e.srv.WritesSince(n)
	if len(writes) != 2 || !strings.HasSuffix(writes[0], `{"location":{"displayName":"Room 4"}}`) ||
		!strings.HasSuffix(writes[1], `{"subject":"Review Q4"}`) {
		t.Errorf("writes = %q", writes)
	}
	doc := e.srv.Event("AAMk-single")
	if loc, _ := doc["location"].(map[string]any); loc["displayName"] != "Room 4" || doc["subject"] != "Review Q4" {
		t.Errorf("Graph's event = %v", doc)
	}
	if r := e.refused("calendar.refused"); len(r) != 0 {
		t.Errorf("refused = %v", r)
	}
	if obj := e.object(e.event("Review Q4")); obj.ETag != e.etag("AAMk-single") || !strings.Contains(string(obj.Remote), `"Review Q4"`) {
		t.Errorf("the object here = etag %q, remote %s", obj.ETag, obj.Remote)
	}
}
