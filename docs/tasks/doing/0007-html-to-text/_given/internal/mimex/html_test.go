package mimex

// CONTRACT TEST for task card T-0007 (docs/tasks). Do not edit.

import (
	"strings"
	"testing"
)

func TestHTMLToText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"paragraphs", `<p>Hello <b>world</b></p><p>Second</p>`, "Hello world\n\nSecond"},
		{"divs", `<div>a</div><div>b</div>`, "a\nb"},
		{"breaks", `line1<br>line2<br/>line3`, "line1\nline2\nline3"},
		{"list", `<ul><li>one</li><li>two</li></ul>`, "- one\n- two"},
		{"entities", `Fish &amp; chips&nbsp;&mdash; &lt;tag&gt; &#169; &copy;`, "Fish & chips — <tag> © ©"},
		{"hidden", `<html><head><title>T</title><style>p{color:red}</style></head><body><script>alert(1)</script><noscript>no js</noscript>Visible<template><p>tpl</p></template></body></html>`, "Visible"},
		{"link", `Click <a href="https://x.test">here</a>.`, "Click here."},
		{"image", `<img src="x.png" alt="logo">Text`, "Text"},
		{"whitespace", "  lots\n\n of\t\tspace  ", "lots of space"},
		{"adjacent spaces", `a <b> b </b> c`, "a b c"},
		{"table", `<table><tr><td>Name</td><td>Value</td></tr><tr><td>A</td><td>1</td></tr></table>`, "Name Value\nA 1"},
		{"comment", `a<!-- hidden -->b`, "ab"},
		{"heading", `<h1>Title</h1>Body`, "Title\n\nBody"},
		{"blockquote", `<p>Reply</p><blockquote><p>Quoted</p></blockquote>`, "Reply\n\nQuoted"},
		{"uppercase tags", `<P>Hi</P><BR>there`, "Hi\n\nthere"},
		{"unclosed", `<p>unclosed <b>bold`, "unclosed bold"},
		{"many breaks", `a<br><br><br><br>b`, "a\n\nb"},
		{"empty", ``, ""},
	}
	for _, tc := range cases {
		if got := HTMLToText(tc.in); got != tc.want {
			t.Errorf("%s: HTMLToText(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestHTMLToTextNewsletter(t *testing.T) {
	in := `<!doctype html><html><body style="margin:0">
<table width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">
<table width="600"><tr><td style="font-family:Helvetica,Arial,sans-serif">
<h1 style="color:#c0392b">October deals</h1>
<p>Up to 40% off. <a href="https://shop.mailtest.test/oct">Shop now</a></p>
<img src="https://shop.mailtest.test/hero.jpg" width="600" alt="Hero">
<script>alert("scripts must never run")</script>
</td></tr></table></td></tr></table>
<img src="https://track.mailtest.test/open.gif?u=1" width="1" height="1" alt="">
</body></html>`
	got := HTMLToText(in)
	if got != "October deals\n\nUp to 40% off. Shop now" {
		t.Fatalf("newsletter = %q", got)
	}
	if strings.Contains(got, "alert") || strings.Contains(got, "http") {
		t.Fatalf("newsletter text leaks script or URLs: %q", got)
	}
}
