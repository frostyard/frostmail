//go:build !windows

package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/store"
)

// apiCode is the API code of err, 0 when it is not an API error.
func apiCode(err error) api.ErrorCode {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

// countingSync counts the body fetches a read starts; only the two fetch
// methods are ever called.
type countingSync struct {
	Syncer
	body, display atomic.Int32
}

func (c *countingSync) FetchBody(context.Context, int64) (string, error) {
	c.body.Add(1)
	return "", errors.New("the server is unreachable in this test")
}

func (c *countingSync) FetchDisplay(context.Context, int64) (string, error) {
	c.display.Add(1)
	return "", errors.New("the server is unreachable in this test")
}

// namedContent is lodge's content route: it names the page, part and source
// paths a client would get.
type namedContent struct{}

func (namedContent) KeepPage([]byte) string           { return "page/1" }
func (namedContent) PartPath(rel string) string       { return "parts/" + rel }
func (namedContent) SourcePath(_, name string) string { return "source/" + name }

// TestReadReportsAStorageFault: a blob store that cannot stat a message's
// stored body is a storage error from message.body, message.source and
// message.render, never "cannot be fetched now", and it starts no fetch.
func TestReadReportsAStorageFault(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its mode")
	}
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	blobs := blob.New(filepath.Join(dir, "blobs"))
	const raw = "From: Ann <ann@mailtest.test>\r\nSubject: Unreadable\r\nContent-Type: text/plain\r\n\r\nHello.\r\n"
	blobID, err := blobs.Put(ctx, strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var stored, bare int64
	err = db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "test1@mailtest.test", Auth: api.AuthKindPassword,
			IMAP: store.ServerConfig{Host: "mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: "test1@mailtest.test"},
			SMTP: store.ServerConfig{Host: "mailtest.test", Port: 587, TLS: api.TLSModeStartTLS, Username: "test1@mailtest.test"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
		ids, err := tx.InsertHeaders(ctx, a.ID, mbs[0].ID, []store.MessageHeader{
			{UID: 1, InternalDate: day, Subject: "Unreadable"},
			{UID: 2, InternalDate: day, Subject: "Not fetched yet"},
		})
		if err != nil {
			return err
		}
		stored, bare = ids[0], ids[1]
		return tx.SetBody(ctx, stored, blobID)
	})
	if err != nil {
		t.Fatal(err)
	}
	sync := &countingSync{}
	msgs := New(Deps{DB: db, Blobs: blobs, Sync: sync, Content: namedContent{},
		Render: &render.Renderer{Parts: &render.PartsCache{Root: filepath.Join(dir, "parts")}}}).Messages()
	content := true

	// With the blob readable, the body is there and source names it.
	if got, err := msgs.Body(ctx, &api.MessageBodyParams{ID: stored}); err != nil || got.Text != "Hello." {
		t.Fatalf("body = %+v, %v; want the stored text", got, err)
	}
	src, err := msgs.Source(ctx, &api.MessageSourceParams{ID: stored, Content: &content})
	if err != nil || src.ContentPath == nil || *src.ContentPath != "source/Unreadable.eml" {
		t.Fatalf("source = %+v, %v; want its content path", src, err)
	}

	// A directory the user may not search makes the blob's stat fail with
	// EACCES, not with "not exist".
	sub := filepath.Join(dir, "blobs", blobID[:2])
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	for _, call := range []struct {
		name string
		call func() error
	}{
		{"message.body", func() error { _, err := msgs.Body(ctx, &api.MessageBodyParams{ID: stored}); return err }},
		{"message.render", func() error { _, err := msgs.Render(ctx, &api.MessageRenderParams{ID: stored}); return err }},
		{"message.source", func() error {
			_, err := msgs.Source(ctx, &api.MessageSourceParams{ID: stored, Content: &content})
			return err
		}},
	} {
		err := call.call()
		if err == nil {
			t.Errorf("%s: no error from an unreadable blob store", call.name)
			continue
		}
		if apiCode(err) == api.CodeUnavailable || strings.Contains(err.Error(), "cannot be fetched now") {
			t.Errorf("%s = %v; want a storage error, not a fetch that failed", call.name, err)
		}
	}
	if n := sync.body.Load(); n != 0 {
		t.Errorf("the storage fault started %d whole-body fetches, want 0", n)
	}
	if n := sync.display.Load(); n != 0 {
		t.Errorf("the storage fault started %d display fetches, want 0", n)
	}

	// A message with no stored body still fetches, as before.
	if _, err := msgs.Body(ctx, &api.MessageBodyParams{ID: bare}); apiCode(err) != api.CodeUnavailable ||
		!strings.Contains(err.Error(), "cannot be fetched now") {
		t.Errorf("a message with no stored body = %v; want unavailable", err)
	}
	if n := sync.display.Load(); n != 1 {
		t.Errorf("display fetches = %d, want 1", n)
	}
}

// TestReadKeepsTheAnswerForADBFault: a message row that cannot be read keeps
// the answer clients had before the narrow read, unavailable with the same
// words, and starts no fetch, though the blob store is there.
func TestReadKeepsTheAnswerForADBFault(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var id int64
	err = db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "test1@mailtest.test", Auth: api.AuthKindPassword,
			IMAP: store.ServerConfig{Host: "mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: "test1@mailtest.test"},
			SMTP: store.ServerConfig{Host: "mailtest.test", Port: 587, TLS: api.TLSModeStartTLS, Username: "test1@mailtest.test"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		ids, err := tx.InsertHeaders(ctx, a.ID, mbs[0].ID, []store.MessageHeader{
			{UID: 1, InternalDate: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Subject: "Unreadable"}})
		if err != nil {
			return err
		}
		id = ids[0]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	sync := &countingSync{}
	msgs := New(Deps{DB: db, Blobs: blob.New(filepath.Join(dir, "blobs")), Sync: sync}).Messages()
	if _, err := msgs.Body(ctx, &api.MessageBodyParams{ID: id}); apiCode(err) != api.CodeUnavailable ||
		!strings.Contains(err.Error(), "is not stored locally and cannot be fetched now") {
		t.Errorf("body with a closed store = %v; want unavailable with the old words", err)
	}
	if n := sync.body.Load() + sync.display.Load(); n != 0 {
		t.Errorf("the read started %d fetches, want 0", n)
	}
}
