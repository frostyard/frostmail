package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestPageOfTheHostileCorpus wraps each hostile message in the reader's
// page, as lodge's content route serves it (docs/specs/lodge-app.md), and
// checks that the page adds only its template to what the sanitizer let
// through: a charset, a color scheme, links into new tabs and the page's
// style, and no script, form or URL to load.
func TestPageOfTheHostileCorpus(t *testing.T) {
	files, err := filepath.Glob("testdata/hostile/*.html")
	if err != nil || len(files) < 40 {
		t.Fatalf("hostile corpus: %d files, %v", len(files), err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var r testResolver
			out, err := sanitizeHTML(string(src), r.resolve)
			if err != nil {
				t.Fatal(err)
			}
			page := string(Page(out))
			doc, err := html.Parse(strings.NewReader(page))
			if err != nil {
				t.Fatal(err)
			}
			var head, body *html.Node
			for n := range doc.Descendants() {
				if n.Type == html.ElementNode && n.Data == "head" {
					head = n
				}
				if n.Type == html.ElementNode && n.Data == "body" {
					body = n
				}
			}
			if head == nil || body == nil {
				t.Fatalf("no head or body:\n%s", page)
			}
			var got []string
			for c := head.FirstChild; c != nil; c = c.NextSibling {
				var b strings.Builder
				_ = html.Render(&b, c)
				got = append(got, b.String())
			}
			want := []string{
				`<meta charset="utf-8"/>`,
				`<meta name="color-scheme" content="light"/>`,
				`<base target="_blank"/>`,
				`<style>` + pageStyle + `</style>`,
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("head:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			var inner strings.Builder
			for c := body.FirstChild; c != nil; c = c.NextSibling {
				_ = html.Render(&inner, c)
			}
			checkSafe(t, inner.String())
		})
	}
}
