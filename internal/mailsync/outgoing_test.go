package mailsync_test

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/compose"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/store"
)

// The reader embeds the received CID image; replying/forwarding must turn
// that representation back into a related part without changing the reader.
func TestDraftMessageSendsEmbeddedImagesAsRelatedParts(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFElEQVQImWP8z8Dwn4GBgYGJAQoAHgQCAZ7JK1kAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	cache := &render.PartsCache{Root: t.TempDir()}
	var received bytes.Buffer
	if err := compose.Build(&received, compose.Message{
		From: compose.Address{Addr: "sender@x.test"}, To: []compose.Address{{Addr: "me@x.test"}},
		Date: time.Now(), MessageID: "received@x.test", HTML: `<img src="cid:picture@x.test" width="20" height="20">`,
		Inline: []compose.Inline{{CID: "picture@x.test", ContentType: "image/png", Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	r := &render.Renderer{Parts: cache}
	shown, err := r.Render(t.Context(), 1, received.Bytes(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shown.HTML, "data:image/png;base64,") {
		t.Fatalf("reader no longer embeds its image: %s", shown.HTML)
	}
	for _, withCache := range []bool{false, true} {
		name := "without cache"
		var parts *render.PartsCache
		if withCache {
			name, parts = "with cache", cache
		}
		t.Run(name, func(t *testing.T) {
			d := store.Draft{MessageID: "forward@x.test", Content: store.DraftContent{
				To: []store.Address{{Addr: "recipient@x.test"}}, HTML: "<blockquote>" + shown.HTML + shown.HTML + "</blockquote>",
			}}
			msg := mailsync.DraftMessage(d, store.Identity{Email: "me@x.test"}, nil, parts, time.Now())
			if len(msg.Inline) != 1 || strings.Count(msg.HTML, "cid:"+msg.Inline[0].CID) != 2 {
				t.Fatalf("expected one related image used twice: %+v; %s", msg.Inline, msg.HTML)
			}
			var sent bytes.Buffer
			if err := compose.Build(&sent, msg); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(sent.String(), "multipart/related") {
				t.Fatal("no multipart/related")
			}
			var html, cid string
			var images int
			if err := mimex.WalkParts(sent.Bytes(), func(p mimex.PartInfo, body io.Reader) error {
				b, err := io.ReadAll(body)
				if p.ContentType == "text/html" {
					html = string(b)
				}
				if p.ContentType == "image/png" {
					images++
					cid = p.ContentID
					if !bytes.Equal(b, data) {
						t.Error("sent image bytes differ")
					}
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if images != 1 || cid == "" || !strings.Contains(html, "cid:"+cid) || strings.Contains(html, "data:image/") {
				t.Fatalf("sent images=%d, CID=%q, HTML=%s", images, cid, html)
			}
			if !strings.Contains(d.Content.HTML, "data:image/png;base64,") {
				t.Fatal("building changed the stored draft")
			}
		})
	}
}

func TestDraftMessageConvertsOnlyValidImageSources(t *testing.T) {
	data := "data:image/png;base64,cGljdHVyZQ=="
	untouched := `<p>` + data + `</p><a href="` + data + `">link</a><!-- <img src="` + data + `"> -->` +
		`<img src="data:image/png;base64,broken!"><img src="data:image/png;base64,"><img src="data:text/plain;base64,dGV4dA=="><img src="https://x.test/image.png">`
	m := mailsync.DraftMessage(store.Draft{Content: store.DraftContent{HTML: untouched}}, store.Identity{Email: "me@x.test"}, nil, nil, time.Now())
	if m.HTML != untouched || len(m.Inline) != 0 {
		t.Fatalf("changed non-image or invalid sources: %s; %+v", m.HTML, m.Inline)
	}
	cache := &render.PartsCache{Root: t.TempDir()}
	if err := cache.Write("m/1/2.png", []byte("cached")); err != nil {
		t.Fatal(err)
	}
	src := `<img src='DATA:IMAGE/PNG;BASE64,cGljdHVyZQ==' alt='a &amp; b'><img src="mailpart://localhost/m/1/2.png"><img src="mailpart://localhost/missing.png">`
	m = mailsync.DraftMessage(store.Draft{Content: store.DraftContent{HTML: src}}, store.Identity{Email: "me@x.test"}, nil, cache, time.Now())
	if len(m.Inline) != 2 || strings.Count(m.HTML, "cid:") != 2 || strings.Contains(m.HTML, "mailpart:") || !strings.Contains(m.HTML, `alt="a &amp; b"`) {
		t.Fatalf("mixed sources: %s; %+v", m.HTML, m.Inline)
	}
}
