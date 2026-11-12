package render

import (
	"encoding/base64"
	"strings"
	"testing"
)

// cidMessage is an HTML part with an image part whose Content-ID is
// <logo@x>.
func cidMessage(t *testing.T, html string) []byte {
	t.Helper()
	img := base64.StdEncoding.EncodeToString(pngBytes(t))
	return []byte(strings.Join([]string{
		"From: News <news@example.test>",
		"Subject: Logo",
		"MIME-Version: 1.0",
		`Content-Type: multipart/related; boundary="rel"`,
		"",
		"--rel",
		"Content-Type: text/html; charset=utf-8",
		"",
		html,
		"--rel",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-ID: <logo@x>",
		"",
		img,
		"--rel--",
		"",
	}, "\r\n"))
}

// TestRenderEmbedsEveryCIDForm: a cid: URL reaches the resolver only after
// the sanitizer has decoded it, so an entity or a CSS escape hides it from
// the message source as written; every form still embeds its image.
func TestRenderEmbedsEveryCIDForm(t *testing.T) {
	img := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t))
	for _, tc := range []struct{ name, html string }{
		{"entity colon", `<p><img src="cid&#58;logo@x" alt="Logo"></p>`},
		{"entity letter", `<p><img src="&#99;id:logo@x" alt="Logo"></p>`},
		{"css escape", `<div style="background:url(\63 id:logo@x)">Logo</div>`},
		{"uppercase", `<p><img src="CID:logo@x" alt="Logo"></p>`},
	} {
		r := &Renderer{Parts: &PartsCache{Root: t.TempDir()}}
		got, err := r.Render(t.Context(), 7, cidMessage(t, tc.html), false)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !strings.Contains(got.HTML, img) {
			t.Errorf("%s: the image is not embedded:\n%s", tc.name, got.HTML)
		}
	}
}

// TestRenderMessageWithoutACIDURL: a message that names no cid: URL renders
// its text, though it carries an image part with a Content-ID.
func TestRenderMessageWithoutACIDURL(t *testing.T) {
	r := &Renderer{Parts: &PartsCache{Root: t.TempDir()}}
	got, err := r.Render(t.Context(), 7, cidMessage(t, `<p>Hello <b>there</b></p>`), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Hello there" || strings.Contains(got.HTML, "data:image") {
		t.Errorf("rendering = %+v; want the text and no image", got)
	}
}
