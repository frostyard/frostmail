package engine_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// newDraft starts a draft on a fresh account, over c.
func newDraft(t *testing.T, c *api.Client) *api.Draft {
	t.Helper()
	ctx := t.Context()
	if _, err := c.Account().Create(ctx, createParams("me@mailtest.test")); err != nil {
		t.Fatal(err)
	}
	d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func chunk(id int64, name string, size, offset int64, data []byte, upload *string) *api.DraftAttachDataParams {
	return &api.DraftAttachDataParams{ID: id, Filename: name, Size: size, Offset: offset,
		Data: base64.StdEncoding.EncodeToString(data), UploadID: upload}
}

// TestAttachDataInChunks: a browser's file arrives in chunks and becomes the
// draft's attachment, as draft.attach makes one from a path.
func TestAttachDataInChunks(t *testing.T) {
	srv := rpctest.Start(t)
	c, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindSession, Login: "e2e@example.com", NodeName: "e2e"})
	d := newDraft(t, c)
	ctx := t.Context()
	file := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 3*engine.MaxUploadChunk/2)...)
	size := int64(len(file))

	first, err := c.Draft().AttachData(ctx, chunk(d.ID, "Report.pdf", size, 0, file[:engine.MaxUploadChunk], nil))
	if err != nil {
		t.Fatal(err)
	}
	if first.Attachment != nil || first.Received != engine.MaxUploadChunk || first.UploadID == "" {
		t.Fatalf("first chunk = %+v, want the upload, part received", first)
	}
	id := first.UploadID
	if _, err := c.Draft().AttachData(ctx, chunk(d.ID, "Report.pdf", size, 5, file[5:10], &id)); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a chunk at the wrong offset = %v, want conflict", err)
	}
	if _, err := c.Draft().AttachData(ctx, chunk(d.ID, "Other.pdf", size, first.Received, file[first.Received:], &id)); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a chunk naming another file = %v, want notFound", err)
	}
	last, err := c.Draft().AttachData(ctx, chunk(d.ID, "Report.pdf", size, first.Received, file[first.Received:], &id))
	if err != nil {
		t.Fatal(err)
	}
	a := last.Attachment
	if a == nil || a.Filename != "Report.pdf" || a.ContentType != "application/pdf" || a.Size != size {
		t.Fatalf("last chunk = %+v, want the attachment", last)
	}
	got, err := c.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID})
	if err != nil || len(got.Attachments) != 1 || got.Attachments[0].ID != a.ID {
		t.Errorf("draft = %+v, %v; want the attachment", got, err)
	}
	if _, err := c.Draft().AttachData(ctx, chunk(d.ID, "Report.pdf", size, last.Received, []byte("x"), &id)); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a chunk after the end = %v, want notFound", err)
	}
}

func TestAttachDataRefuses(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	d := newDraft(t, c)
	ctx := t.Context()
	for name, p := range map[string]*api.DraftAttachDataParams{
		"a path":             chunk(d.ID, "../etc/passwd", 4, 0, []byte("root"), nil),
		"a Windows path":     chunk(d.ID, `C:\x.txt`, 4, 0, []byte("root"), nil),
		"no name":            chunk(d.ID, "  ", 4, 0, []byte("root"), nil),
		"a control name":     chunk(d.ID, "a\nb.txt", 4, 0, []byte("root"), nil),
		"past the limit":     chunk(d.ID, "big.bin", engine.AttachmentLimit+1, 0, []byte("x"), nil),
		"no size":            chunk(d.ID, "a.txt", 0, 0, []byte("x"), nil),
		"an empty chunk":     chunk(d.ID, "a.txt", 4, 0, nil, nil),
		"past its size":      chunk(d.ID, "a.txt", 2, 0, []byte("abc"), nil),
		"a later first":      chunk(d.ID, "a.txt", 4, 2, []byte("ab"), nil),
		"an oversized chunk": chunk(d.ID, "a.bin", engine.MaxUploadChunk+1, 0, make([]byte, engine.MaxUploadChunk+1), nil),
	} {
		if _, err := c.Draft().AttachData(ctx, p); !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("%s: %v, want invalidParams", name, err)
		}
	}
	bad := &api.DraftAttachDataParams{ID: d.ID, Filename: "a.txt", Size: 4, Data: "not base64!"}
	if _, err := c.Draft().AttachData(ctx, bad); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("data that is not base64: %v, want invalidParams", err)
	}
	if _, err := c.Draft().AttachData(ctx, chunk(d.ID+99, "a.txt", 1, 0, []byte("a"), nil)); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("another draft: %v, want notFound", err)
	}
	for i := range 4 {
		if _, err := c.Draft().AttachData(ctx, chunk(d.ID, "a.txt", 4, 0, []byte("a"), nil)); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	if _, err := c.Draft().AttachData(ctx, chunk(d.ID, "a.txt", 4, 0, []byte("a"), nil)); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a fifth upload on one connection: %v, want conflict", err)
	}
}

// TestAttachDataBelongsToItsConnection: another connection cannot continue
// an upload, and a connection's end discards its uploads.
func TestAttachDataBelongsToItsConnection(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	d := newDraft(t, c)
	ctx := t.Context()
	up, err := c.Draft().AttachData(ctx, chunk(d.ID, "a.txt", 4, 0, []byte("ab"), nil))
	if err != nil {
		t.Fatal(err)
	}
	other := srv.Dial(t)
	if _, err := other.Draft().AttachData(ctx, chunk(d.ID, "a.txt", 4, 2, []byte("cd"), &up.UploadID)); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("another connection's chunk = %v, want notFound", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := other.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID})
	if err != nil || len(got.Attachments) != 0 {
		t.Errorf("draft = %+v, %v; want no attachment from an unfinished upload", got, err)
	}
}

func TestAgentsCannotUpload(t *testing.T) {
	srv := rpctest.Start(t)
	d := newDraft(t, srv.Dial(t))
	s, err := api.ParseScope("account=*")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindAgent, Token: "claude", Scope: s})
	_, err = a.Draft().AttachData(t.Context(), chunk(d.ID, "a.txt", 1, 0, []byte("a"), nil))
	if !errors.Is(err, api.ErrMethodNotFound) || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("draft.attachData as an agent = %v, want methodNotFound", err)
	}
}
