package notify

import (
	"encoding/json/v2"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestBroadcastPublishesTheDesktopsNotes(t *testing.T) {
	var got []api.EventEnvelope
	announce := Broadcast(func(evs []api.EventEnvelope) { got = append(got, evs...) })
	if err := announce(t.Context(), 4, nil); err != nil || len(got) != 0 {
		t.Fatalf("no mail: %v, %d events; want none", err, len(got))
	}
	mail := []Mail{
		{ID: 11, FromName: "Ann", Subject: "Plan", Preview: "See you"},
		{ID: 12, FromAddr: "bob@mailtest.test", Subject: "Reminder: Lunch"},
	}
	if err := announce(t.Context(), 4, mail); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Event != "message.announced" || got[0].Seq != 0 {
		t.Fatalf("events = %+v, want one transient message.announced", got)
	}
	var ev api.MessageAnnounced
	if err := json.Unmarshal(got[0].Data, &ev); err != nil {
		t.Fatal(err)
	}
	notes := Notes(mail)
	if ev.AccountID != 4 || len(ev.Notes) != len(notes) {
		t.Fatalf("event = %+v, want account 4 and %d notes", ev, len(notes))
	}
	for i, n := range notes {
		a := ev.Notes[i]
		if a.Title != n.Summary || a.Body != n.Body || a.MessageID == nil || *a.MessageID != n.MessageID {
			t.Errorf("note %d = %+v, want %+v", i, a, n)
		}
	}

	got = nil
	group := []Mail{{ID: 1, FromName: "A"}, {ID: 2, FromName: "B"}, {ID: 3, FromName: "C"}, {ID: 4, FromName: "D"}}
	if err := announce(t.Context(), 4, group); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got[0].Data, &ev); err != nil {
		t.Fatal(err)
	}
	if len(ev.Notes) != 1 || ev.Notes[0].Title != "4 new messages" || ev.Notes[0].MessageID != nil {
		t.Errorf("a group = %+v, want one note opening no message", ev.Notes)
	}
}
