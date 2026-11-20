package engine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func newSizeDraft(t *testing.T, dataDir string) (*rpctest.Server, *api.Client, *api.Draft) {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{DataDir: dataDir})
	c := srv.Dial(t)
	dr := newDraft(t, c)
	content := dr.Content
	content.To = []api.Address{{Address: "ada@example.com"}}
	content.HTML = "<p>" + strings.Repeat("x", 2048) + "</p>"
	updated, err := c.Draft().Update(t.Context(), &api.DraftUpdateParams{ID: dr.ID, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	return srv, c, updated
}

func TestDraftSendKnownSMTPSize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int64
		allow bool
	}{
		{"over", 1_000, false},
		{"under", 10_000_000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			srv, c, dr := newSizeDraft(t, dataDir)
			if err := srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
				return tx.SetSMTPMaxSize(t.Context(), dr.AccountID, tc.limit)
			}); err != nil {
				t.Fatal(err)
			}
			item, err := c.Draft().Send(t.Context(), &api.DraftSendParams{ID: dr.ID})
			if tc.allow {
				if err != nil || item.State != api.OutboxStateQueued {
					t.Fatalf("draft.send = %+v, %v; want queued", item, err)
				}
			} else {
				if code(err) != api.CodeInvalidParams {
					t.Errorf("draft.send = %v, want invalidParams", err)
				} else if !strings.Contains(err.Error(), "the message is 0.0 MB, larger than the server's 0.0 MB limit") {
					t.Errorf("size error = %v, want sizes in MB with 1 decimal", err)
				}
				if _, statErr := os.Stat(filepath.Join(dataDir, "blobs")); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("refused message created a blob directory: %v", statErr)
				}
			}
			rows, err := srv.DB.ListOutbox(t.Context(), dr.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.allow {
				if len(rows) != 1 || rows[0].DraftID != dr.ID || rows[0].State != "queued" {
					t.Errorf("outbox = %+v, want 1 queued row for the draft", rows)
				}
			} else if len(rows) != 0 {
				t.Errorf("outbox has %d rows for a refused draft, want 0", len(rows))
			}
		})
	}
}

func TestDraftSendAttachmentSizeFallback(t *testing.T) {
	for _, limit := range []int64{0, 10_000_000} {
		name := "unknown"
		if limit > 0 {
			name = "known"
		}
		t.Run(name, func(t *testing.T) {
			srv, c, dr := newSizeDraft(t, t.TempDir())
			blobID, err := srv.Blobs.Put(t.Context(), strings.NewReader("attachment"))
			if err != nil {
				t.Fatal(err)
			}
			if err := srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
				if err := tx.SetSMTPMaxSize(t.Context(), dr.AccountID, limit); err != nil {
					return err
				}
				// Inflate only the recorded size to exercise the attachment sum cheaply.
				_, err := tx.AddDraftAttachment(t.Context(), dr.ID, store.DraftAttachment{
					BlobID: blobID, Filename: "large.txt", ContentType: "text/plain", Size: engine.AttachmentLimit + 1,
				})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			_, err = c.Draft().Send(t.Context(), &api.DraftSendParams{ID: dr.ID})
			if limit == 0 {
				if code(err) != api.CodeInvalidParams || !strings.Contains(err.Error(), "the attachments are larger than the 25 MB limit") {
					t.Errorf("draft.send = %v, want the 25 MB attachment error", err)
				}
			} else if err != nil {
				t.Errorf("known SIZE must replace the attachment fallback: %v", err)
			}
		})
	}
}

func TestOutboxRescheduleKnownSMTPSize(t *testing.T) {
	srv, c, dr := newSizeDraft(t, t.TempDir())
	at := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	item, err := c.Draft().Send(t.Context(), &api.DraftSendParams{ID: dr.ID, SendAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
		return tx.SetSMTPMaxSize(t.Context(), dr.AccountID, 1_000)
	}); err != nil {
		t.Fatal(err)
	}
	_, err = c.Outbox().Reschedule(t.Context(), &api.OutboxRescheduleParams{ID: item.ID, SendAt: at.Add(time.Hour)})
	if code(err) != api.CodeInvalidParams {
		t.Errorf("outbox.reschedule = %v, want invalidParams", err)
	}
	row, err := srv.DB.GetOutbox(t.Context(), item.ID)
	if err != nil || !row.SendAt.Equal(at) || row.State != "queued" {
		t.Errorf("outbox changed after refusal: %+v, %v", row, err)
	}
}
