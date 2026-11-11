package engine_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestDraftImageNamesOnlyReferencedCachedImages(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Lodge: true})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Account().Create(ctx, createParams("me@mailtest.test")); err != nil {
		t.Fatal(err)
	}
	draft, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	const rel = "r/0123456789abcdef0123456789abcdef.png"
	if err := srv.Parts.Write(rel, b.Bytes()); err != nil {
		t.Fatal(err)
	}
	setBody := func(body string) {
		t.Helper()
		content := draft.Content
		content.HTML = body
		if _, err := c.Draft().Update(ctx, &api.DraftUpdateParams{ID: draft.ID, Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	src := "mailpart://localhost/" + rel
	setBody(`<p><IMG SRC="` + src + `"></p>`)
	f, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: rel})
	if err != nil {
		t.Fatal(err)
	}
	if f.ContentPath == nil || f.Path != rel || f.ContentType != "image/png" || f.Size != int64(b.Len()) {
		t.Fatalf("image=%+v", f)
	}
	session, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindSession, Login: "test"})
	if _, err := session.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: rel}); err != nil {
		t.Fatalf("session: %v", err)
	}
	agent, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindAgent, Token: "test", Scope: api.Scope{All: true, Write: true}})
	if _, err := agent.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: rel}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Fatalf("agent: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Content.ServeHTTP(rec, httptest.NewRequest("GET", "/content/"+*f.ContentPath, nil))
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), b.Bytes()) {
		t.Fatalf("content=%d %q", rec.Code, rec.Body)
	}
	for _, body := range []string{"", `<a href="` + src + `">link</a>`, `<!-- <img src="` + src + `"> -->`, `<script>var x='<img src="` + src + `">';</script>`} {
		setBody(body)
		if _, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: rel}); err == nil {
			t.Errorf("accepted body %q", body)
		}
	}
	for _, bad := range []string{"../outside.png", "/tmp/outside.png", `r\outside.png`, "r//x.png", "."} {
		setBody(`<img src="mailpart://localhost/` + bad + `">`)
		if _, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: bad}); err == nil {
			t.Errorf("accepted path %q", bad)
		}
	}
	for name, data := range map[string][]byte{"text.png": []byte("not an image"), "huge.png": append(bytes.Clone(b.Bytes()), make([]byte, render.MaxImageBytes)...)} {
		rel := "r/" + name
		if err := srv.Parts.Write(rel, data); err != nil {
			t.Fatal(err)
		}
		setBody(`<img src="mailpart://localhost/` + rel + `">`)
		if _, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: rel}); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(srv.Parts.Root, "r", "link.png")); err == nil {
		setBody(`<img src="mailpart://localhost/r/link.png">`)
		if _, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: "r/link.png"}); err == nil {
			t.Error("accepted escaping symlink")
		}
	}
	setBody(`<img src="mailpart://localhost/r/missing.png">`)
	if _, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID, Path: "r/missing.png"}); err == nil {
		t.Error("accepted missing image")
	}
	var rpcErr *api.Error
	if _, err := c.Draft().Image(ctx, &api.DraftImageParams{ID: draft.ID + 1, Path: rel}); !errors.As(err, &rpcErr) || rpcErr.Code != api.CodeNotFound {
		t.Errorf("missing draft: %v", err)
	}
}
