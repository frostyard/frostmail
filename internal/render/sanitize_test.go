package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// allowedURL is what a resolver in these tests may return.
var allowedURL = regexp.MustCompile(`^(mailpart://localhost/[A-Za-z0-9._/-]+|data:image/(png|gif|jpeg|webp);base64,[A-Za-z0-9+/=\s]+)$`)

var safeCID = regexp.MustCompile(`^cid:[A-Za-z0-9._-]+$`)

// testResolver keeps safe cid: URLs as mailpart URLs, as Renderer does with
// part paths, and drops everything else, counting what it saw.
type testResolver struct{ remote, trackers int }

func (r *testResolver) resolve(raw string, tracker bool) string {
	raw = strings.TrimSpace(raw)
	switch {
	case safeCID.MatchString(raw) && raw != "cid:none":
		return "mailpart://localhost/m/1/" + strings.TrimPrefix(raw, "cid:") + ".png"
	case strings.HasPrefix(raw, "data:image/png;base64,"):
		return raw
	case tracker:
		r.trackers++
	case strings.HasPrefix(raw, "http"):
		r.remote++
	}
	return ""
}

// checkSafe fails the test unless out, reparsed, holds only allowlisted
// elements and attributes, links only to http(s) and mailto, and loads
// nothing but mailpart and data images.
func checkSafe(t *testing.T, out string) {
	t.Helper()
	ctx := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(out), ctx)
	if err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, out)
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Namespace != "" || !keptElements[n.Data] {
				t.Errorf("element <%s> in output:\n%s", n.Data, out)
			}
			if n.Data == "style" {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					checkCSS(t, c.Data, out)
				}
			}
			for _, a := range n.Attr {
				switch {
				case a.Key == "style":
					checkCSS(t, a.Val, out)
				case a.Key == "href" && n.Data == "a":
					if linkURL(a.Val) != a.Val {
						t.Errorf("href %q in output", a.Val)
					}
				case a.Key == "src" && n.Data == "img", a.Key == "background":
					if !allowedURL.MatchString(a.Val) {
						t.Errorf("%s=%q in output", a.Key, a.Val)
					}
				case !keptAttributes[a.Key]:
					t.Errorf("attribute %s on <%s> in output", a.Key, n.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
}

// checkCSS lexes sanitized CSS again and fails on anything that could load
// or run: loading at-rules, url() with a URL that is not allowed, blocked
// functions and properties, and "<".
func checkCSS(t *testing.T, src, out string) {
	t.Helper()
	if strings.Contains(src, "<") {
		t.Errorf("CSS contains '<':\n%s", out)
	}
	l := css.NewLexer(parse.NewInputString(src))
	for {
		tt, data := l.Next()
		switch tt {
		case css.ErrorToken:
			return
		case css.URLToken:
			if u := urlTokenValue(data); !allowedURL.MatchString(u) {
				t.Errorf("CSS url %q in output:\n%s", u, out)
			}
		case css.BadURLToken:
			t.Errorf("bad url token %q in CSS:\n%s", data, out)
		case css.AtKeywordToken:
			switch strings.ToLower(cssUnescape(string(data[1:]))) {
			case "import", "font-face", "namespace", "charset":
				t.Errorf("at-rule %q in CSS:\n%s", data, out)
			}
		case css.IdentToken:
			switch strings.ToLower(cssUnescape(string(data))) {
			case "behavior", "-moz-binding":
				t.Errorf("property %q in CSS:\n%s", data, out)
			}
		case css.FunctionToken:
			name := strings.ToLower(cssUnescape(string(data[:len(data)-1])))
			if blockedFunctions[name] {
				t.Errorf("function %q in CSS:\n%s", data, out)
			}
			if name == "url" {
				if u := functionArgument(l); !allowedURL.MatchString(u) {
					t.Errorf("CSS url %q in output:\n%s", u, out)
				}
			}
		}
	}
}

func TestHostileCorpus(t *testing.T) {
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
			checkSafe(t, out)
		})
	}
}

func TestSanitizeKeepsMailLayout(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"tables and presentation attributes",
			`<table width="600" cellpadding="0" bgcolor="#fff" onclick="x()"><tr><td align="center" class="hero">Hi</td></tr></table>`,
			`<table width="600" cellpadding="0" bgcolor="#fff"><tbody><tr><td align="center" class="hero">Hi</td></tr></tbody></table>`,
		},
		{
			"head styles survive, other head content does not",
			`<html><head><title>T</title><style>.a{color:red}</style><meta name="x"></head><body><p class="a">x</p></body></html>`,
			`<style>.a{color:red}</style><p class="a">x</p>`,
		},
		{
			"body presentation moves to a wrapper",
			`<body bgcolor="#eee" style="margin:0" onload="x()"><p>x</p></body>`,
			`<div bgcolor="#eee" style="margin:0"><p>x</p></div>`,
		},
		{
			"links keep http, https and mailto",
			`<a href="https://e.test/a?b=1&amp;c=2" target="_blank">a</a><a href="mailto:x@e.test">b</a><a href="/rel">c</a><a href="#top">d</a>`,
			`<a href="https://e.test/a?b=1&amp;c=2">a</a><a href="mailto:x@e.test">b</a><a>c</a><a>d</a>`,
		},
		{
			"unknown elements are unwrapped, dangerous ones dropped with content",
			`<o:p>keep</o:p><custom-el>also</custom-el><script>gone</script><form><input>gone</form>`,
			`keepalso`,
		},
		{
			"cid images resolve, remote ones are left out",
			`<img src="cid:logo" alt="Logo" width="80"><img src="https://e.test/a.png" alt="A">`,
			`<img src="mailpart://localhost/m/1/logo.png" alt="Logo" width="80"/><img alt="A"/>`,
		},
		{
			"inline styles keep layout and lose urls",
			`<table><tr><td style="padding:4px;background:url(https://e.test/bg.png) no-repeat">x</td></tr></table>`,
			`<table><tbody><tr><td style="padding:4px;background:none no-repeat">x</td></tr></tbody></table>`,
		},
		{
			"comments go",
			`<p>a<!-- secret --></p>`,
			`<p>a</p>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r testResolver
			got, err := sanitizeHTML(tc.in, r.resolve)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("sanitize(%q)\n got %s\nwant %s", tc.in, got, tc.want)
			}
			checkSafe(t, got)
		})
	}
}

func TestTrackingImages(t *testing.T) {
	cases := []struct {
		img     string
		tracker bool
	}{
		{`<img src="https://e.test/p.gif" width="1" height="1">`, true},
		{`<img src="https://e.test/p.gif" width="2px" height="0">`, true},
		{`<img src="https://e.test/p.gif" style="width:1px;height:1px">`, true},
		{`<img src="https://e.test/p.gif" style="display: none">`, true},
		{`<img src="https://e.test/p.gif" style="visibility:hidden !important">`, true},
		{`<img src="https://e.test/p.gif" style="opacity:0">`, true},
		{`<img src="https://e.test/p.gif" width="1" height="40">`, false},
		{`<img src="https://e.test/p.gif" width="1">`, false},
		{`<img src="https://e.test/p.gif">`, false},
	}
	for _, tc := range cases {
		var r testResolver
		if _, err := sanitizeHTML(tc.img, r.resolve); err != nil {
			t.Fatal(err)
		}
		if got := r.trackers == 1; got != tc.tracker {
			t.Errorf("%s: tracker = %v, want %v", tc.img, got, tc.tracker)
		}
	}
}

func FuzzSanitizeHTML(f *testing.F) {
	files, _ := filepath.Glob("testdata/hostile/*.html")
	for _, file := range files {
		if b, err := os.ReadFile(file); err == nil {
			f.Add(string(b))
		}
	}
	f.Add(`<table style="background:\75 rl(x)"><td background="cid:a">`)
	f.Fuzz(func(t *testing.T, src string) {
		var r testResolver
		out, err := sanitizeHTML(src, r.resolve)
		if err != nil {
			return
		}
		checkSafe(t, out)
	})
}
