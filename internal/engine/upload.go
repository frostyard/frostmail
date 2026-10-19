package engine

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// draft.attachData (docs/specs/lodge-app.md, Methods and callers): a file
// sent as bytes, for a client that cannot name a path on maild's machine.
// The chunks go to a private temporary file, which becomes a blob and an
// attachment when the last one is in. An upload belongs to the connection
// that started it and ends with it.

// Limits on uploads.
const (
	// MaxUploadChunk is the most one chunk carries, decoded: its base64
	// stays well within a 16 MiB message.
	MaxUploadChunk = 4 << 20
	// maxUploadsPerConn and maxUploads bound the uploads in progress.
	maxUploadsPerConn = 4
	maxUploads        = 16
	// uploadIdle discards an upload that has sent nothing for this long.
	uploadIdle = 10 * time.Minute
)

// upload is one file arriving in chunks.
type upload struct {
	id       string
	conn     api.Conn
	draft    int64
	filename string
	size     int64
	received int64
	seen     time.Time
	f        *os.File
	done     chan struct{} // closed when the upload ends
}

// end closes and removes the upload's file; it is idempotent.
func (u *upload) end() {
	select {
	case <-u.done:
		return
	default:
	}
	close(u.done)
	_ = u.f.Close()
	_ = os.Remove(u.f.Name())
}

// uploads holds the uploads in progress, shared by every connection.
type uploads struct {
	mu  sync.Mutex
	m   map[string]*upload
	now func() time.Time
	dir string // "" for the system's temporary directory
}

func newUploads() *uploads { return &uploads{m: map[string]*upload{}, now: time.Now} }

// sweep ends uploads idle for uploadIdle; it holds u.mu.
func (s *uploads) sweep() {
	for id, u := range s.m {
		if s.now().Sub(u.seen) >= uploadIdle {
			delete(s.m, id)
			u.end()
		}
	}
}

// start begins an upload for conn, ending it when conn does; put keeps it
// once its first chunk is written.
func (s *uploads) start(conn api.Conn, draft int64, filename string, size int64) (*upload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	mine := 0
	for _, u := range s.m {
		if u.conn == conn {
			mine++
		}
	}
	if mine >= maxUploadsPerConn || len(s.m) >= maxUploads {
		return nil, api.Conflict("too many uploads in progress; finish one first")
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw) // crypto/rand.Read never fails
	f, err := os.CreateTemp(s.dir, "frostmail-upload-*")
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	u := &upload{id: hex.EncodeToString(raw), conn: conn, draft: draft, filename: filename, size: size,
		seen: s.now(), f: f, done: make(chan struct{})}
	go func() {
		select {
		case <-conn.Done():
			s.mu.Lock()
			delete(s.m, u.id)
			s.mu.Unlock()
			u.end()
		case <-u.done:
		}
	}()
	return u, nil
}

// take finds conn's upload id for a draft and file, removing it from the
// map while a chunk is written; put returns it.
func (s *uploads) take(conn api.Conn, id string, draft int64, filename string, size int64) (*upload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	u := s.m[id]
	if u == nil || u.conn != conn || u.draft != draft || u.filename != filename || u.size != size {
		return nil, api.NotFound("upload %q does not exist", id)
	}
	delete(s.m, id)
	return u, nil
}

func (s *uploads) put(u *upload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-u.done: // its connection ended meanwhile
	default:
		u.seen = s.now()
		s.m[u.id] = u
	}
}

// uploadName checks a file name a client sends: a name, not a path.
func uploadName(name string) (string, error) {
	n := strings.TrimSpace(name)
	switch {
	case n == "" || n == "." || n == "..":
		return "", api.InvalidParams("a file name is required")
	case len(n) > 255 || !utf8.ValidString(n):
		return "", api.InvalidParams("the file name is too long or not UTF-8")
	case strings.ContainsAny(n, `/\`) || strings.ContainsFunc(n, unicode.IsControl):
		return "", api.InvalidParams("the file name %q holds a directory or a control character", n)
	}
	return n, nil
}

// AttachData implements draft.attachData.
func (d drafts) AttachData(ctx context.Context, p *api.DraftAttachDataParams) (*api.DraftUpload, error) {
	if d.Blobs == nil {
		return nil, api.Unavailable("the blob store is not running")
	}
	conn := api.ConnFrom(ctx)
	if conn == nil || d.uploads == nil {
		return nil, api.Unavailable("uploads need a connection")
	}
	dr, err := d.DB.GetDraft(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	name, err := uploadName(p.Filename)
	if err != nil {
		return nil, err
	}
	var held int64
	for _, a := range dr.Attachments {
		held += a.Size
	}
	if p.Size < 1 || held+p.Size > AttachmentLimit {
		return nil, api.InvalidParams("%s would take the draft's attachments past the %d MB limit", name, AttachmentLimit/1_000_000)
	}
	if base64.StdEncoding.DecodedLen(len(p.Data)) > MaxUploadChunk+2 {
		return nil, api.InvalidParams("a chunk carries at most %d bytes", MaxUploadChunk)
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil, api.InvalidParams("data is not base64: %v", err)
	}
	if len(data) == 0 || len(data) > MaxUploadChunk {
		return nil, api.InvalidParams("a chunk carries 1 to %d bytes", MaxUploadChunk)
	}

	var u *upload
	if p.UploadID == nil {
		if p.Offset != 0 {
			return nil, api.InvalidParams("an upload starts at offset 0")
		}
		if u, err = d.uploads.start(conn, dr.ID, name, p.Size); err != nil {
			return nil, err
		}
	} else if u, err = d.uploads.take(conn, *p.UploadID, dr.ID, name, p.Size); err != nil {
		return nil, err
	}
	fail := func(err error) (*api.DraftUpload, error) {
		u.end()
		return nil, err
	}
	if p.Offset != u.received {
		d.uploads.put(u)
		return nil, api.Conflict("upload %s holds %d bytes; send the chunk at that offset", u.id, u.received)
	}
	if u.received+int64(len(data)) > u.size {
		return fail(api.InvalidParams("the chunk runs past the file's %d bytes", u.size))
	}
	if _, err := u.f.Write(data); err != nil {
		return fail(fmt.Errorf("upload: %w", err))
	}
	u.received += int64(len(data))
	if u.received < u.size {
		d.uploads.put(u)
		return &api.DraftUpload{UploadID: u.id, Received: u.received}, nil
	}
	a, err := d.attachUpload(ctx, dr, u)
	u.end()
	if err != nil {
		return nil, err
	}
	return &api.DraftUpload{UploadID: u.id, Received: u.received, Attachment: &a}, nil
}

// attachUpload stores a finished upload as the draft's attachment.
func (d drafts) attachUpload(ctx context.Context, dr store.Draft, u *upload) (api.DraftAttachment, error) {
	if _, err := u.f.Seek(0, io.SeekStart); err != nil {
		return api.DraftAttachment{}, err
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(u.f, head)
	if _, err := u.f.Seek(0, io.SeekStart); err != nil {
		return api.DraftAttachment{}, err
	}
	blobID, err := d.Blobs.Put(ctx, u.f)
	if err != nil {
		return api.DraftAttachment{}, err
	}
	a := store.DraftAttachment{BlobID: blobID, Filename: u.filename, ContentType: contentTypeOf(u.filename, head[:n]), Size: u.size}
	err = d.DB.Tx(ctx, func(tx *store.Tx) error {
		if a, err = tx.AddDraftAttachment(ctx, dr.ID, a); err != nil {
			return err
		}
		if err := tx.TouchDraft(ctx, dr.ID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.DraftChanged{ID: dr.ID, AccountID: dr.AccountID})
	})
	if err != nil {
		return api.DraftAttachment{}, apiError(err, fmt.Sprintf("draft %d", dr.ID))
	}
	d.draftsChanged(dr.AccountID)
	return toAPIAttachment(a), nil
}
