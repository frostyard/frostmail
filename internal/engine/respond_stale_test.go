package engine

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
)

// inviteSource is the stored invitation to Launch review, which the user
// has accepted.
const inviteSource = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Frostmail//Test//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:launch@example.com\r\nSEQUENCE:1\r\nDTSTAMP:20261009T140000Z\r\n" +
	"DTSTART:20261015T140000Z\r\nDTEND:20261015T150000Z\r\nSUMMARY:Launch review\r\n" +
	"ORGANIZER;CN=Maria Lopez:mailto:maria@example.com\r\n" +
	"ATTENDEE;CN=Maria Lopez;PARTSTAT=ACCEPTED:mailto:maria@example.com\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@dav.test\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// staleEnv is an iCloud account whose Work calendar holds the invitation,
// with its creation queued as the first answer leaves it, and a service
// that answers it.
type staleEnv struct {
	db      *store.DB
	c       calendarService
	acct    int64
	col     int64
	eventID int64
	objID   int64
	opID    int64
}

func newStaleEnv(t *testing.T) *staleEnv {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &staleEnv{db: db, c: calendarService{Deps{DB: db}}}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		srv := store.ServerConfig{Host: "p01-caldav.icloud.example", Port: 443, TLS: api.TLSModeTLS, Username: "user@dav.test"}
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindICloud, Email: "user@dav.test",
			DisplayName: "User", Auth: api.AuthKindPassword, IMAP: srv, SMTP: srv})
		if err != nil {
			return err
		}
		e.acct = acct.ID
		cols, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar,
			[]store.RemoteCollection{{Href: "/cal/work/", Name: "Work", Components: []string{"VEVENT"}}})
		if err != nil {
			return err
		}
		e.col = cols[0].ID
		id, err := e.put(ctx, tx, inviteSource)
		if err != nil {
			return err
		}
		e.objID = id
		e.opID, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: e.acct, CollectionID: e.col, ObjectID: &id,
			Kind: "put", Href: "/cal/work/launch.ics"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.EventsByUID(ctx, e.acct, "launch@example.com")
	if err != nil || len(rows) != 1 {
		t.Fatalf("stored events = %+v, %v", rows, err)
	}
	e.eventID = rows[0].ID
	return e
}

// put stores src as the invitation's object at its href, as a pass does.
func (e *staleEnv) put(ctx context.Context, tx *store.Tx, src string) (int64, error) {
	from, to := pimsync.Window(tx.Now())
	return pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: e.col, Href: "/cal/work/launch.ics",
		Kind: store.ObjectVEvent, UID: "launch@example.com", Raw: []byte(src)},
		calendar.Options{Local: time.UTC, UserEmails: []string{"user@dav.test"}}, from, to)
}

// read is the response Respond reads for a tentative answer, before any
// write: the state a race makes stale.
func (e *staleEnv) read(t *testing.T) *response {
	t.Helper()
	r, err := e.c.response(t.Context(), &api.CalendarRespondParams{EventID: &e.eventID, Answer: api.PartStatTentative})
	if err != nil {
		t.Fatal(err)
	}
	return &r
}

// TestAnswerKeepsTheETagAPassWrote: an answer written after a pass settled
// the creation keeps the ETag and href the pass wrote, so its queued put
// patches the server's copy instead of creating another.
func TestAnswerKeepsTheETagAPassWrote(t *testing.T) {
	ctx := t.Context()
	e := newStaleEnv(t)
	r := e.read(t)
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetObjectETag(ctx, e.objID, `"e1"`); err != nil {
			return err
		}
		return tx.DonePIMOp(ctx, e.opID)
	}); err != nil {
		t.Fatal(err)
	}
	id, err := e.c.answerStored(ctx, r, false)
	if err != nil || id != e.eventID {
		t.Fatalf("answerStored = %d, %v; want %d", id, err, e.eventID)
	}
	obj, err := e.db.GetObject(ctx, e.objID)
	if err != nil {
		t.Fatal(err)
	}
	if obj.ETag != `"e1"` || !bytes.Contains(obj.Raw, []byte("PARTSTAT=TENTATIVE")) {
		t.Errorf("object etag %q, source:\n%s", obj.ETag, obj.Raw)
	}
	ops, err := e.db.DuePIMOps(ctx, e.acct, e.db.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Kind != "put" || ops[0].ObjectID == nil || *ops[0].ObjectID != e.objID ||
		ops[0].IfMatch != `"e1"` {
		t.Errorf("ops = %+v; want one put on %q", ops, `"e1"`)
	}
}

// TestAnswerOfAChangedSourceIsStale: an answer whose source a pass replaced
// since it was read writes nothing and reports the answer stale, so the
// pass's source stays.
func TestAnswerOfAChangedSourceIsStale(t *testing.T) {
	ctx := t.Context()
	e := newStaleEnv(t)
	r := e.read(t)
	changed := strings.Replace(inviteSource, "SUMMARY:Launch review", "SUMMARY:Launch review (moved)", 1)
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := e.put(ctx, tx, changed)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.answerStored(ctx, r, false); !errors.Is(err, errStale) {
		t.Fatalf("answerStored = %v; want errStale", err)
	}
	obj, err := e.db.GetObject(ctx, e.objID)
	if err != nil {
		t.Fatal(err)
	}
	if string(obj.Raw) != changed {
		t.Errorf("the object's source was written:\n%s", obj.Raw)
	}
}
