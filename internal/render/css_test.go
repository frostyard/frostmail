package render

import (
	"strings"
	"testing"
)

func TestSanitizeCSS(t *testing.T) {
	resolve := func(raw string) string {
		if strings.HasPrefix(raw, "cid:") {
			return "mailpart://localhost/m/1/" + strings.TrimPrefix(raw, "cid:") + ".png"
		}
		return ""
	}
	cases := []struct{ name, in, want string }{
		{"plain declarations", "color: red; margin: 0 auto", "color: red; margin: 0 auto"},
		{"rules and media", "@media (max-width:600px){.a{width:100%!important}}", "@media (max-width:600px){.a{width:100%!important}}"},
		{"cid url", "background:url(cid:logo)", `background:url("mailpart://localhost/m/1/logo.png")`},
		{"quoted cid url", `background:url("cid:logo")`, `background:url("mailpart://localhost/m/1/logo.png")`},
		{"remote url", "background:url(https://e.test/a.png) no-repeat", "background:none no-repeat"},
		{"escaped url function", `background:\75 rl(https://e.test/a.png)`, "background:none"},
		{"import", "@import url(https://e.test/x.css); p{color:red}", "  p{color:red}"},
		{"import string", `@import "https://e.test/x.css"; p{color:red}`, "  p{color:red}"},
		{"font face", "@font-face{font-family:x;src:url(https://e.test/x.woff)} p{font-family:x}", "  p{font-family:x}"},
		{"removal cannot glue tokens", "@@import;import", "@ import"},
		{"comment cannot glue tokens", "@/**/import url(x)", "@ import none"},
		{"expression", "width:expression(alert(1)); color:red", "width:none; color:red"},
		{"image-set", `background-image:image-set("a.png" 1x)`, "background-image:none"},
		{"behavior", "behavior:url(x.htc)", "x-blocked:none"},
		{"escaped behavior", `beh\61vior:url(x.htc)`, "x-blocked:none"},
		{"moz binding", "-moz-binding:url(x.xml)", "x-blocked:none"},
		{"style breakout", `p{content:"</style><b>"}`, `p{content:"\3c /style>\3c b>"}`},
		{"comments and cdo", "<!-- p{color:red} /* x */ -->", "  p{color:red}    "},
		{"unterminated url string", `\75rl("`, "none"},
		{"page", `@page{margin:0;@top-left{content:"From: Bank"}} p{color:red}`, "  p{color:red}"},
		{"page in media", `@media print{@page :first{size:1in}}`, "@media print{ }"},
		{"escaped page", `@pag\65{margin:0}`, " "},
		{"layer block", "@layer print-guard{p{color:red}}", "@layer message-print-guard{p{color:red}}"},
		{"layer statement", "@layer a, print-guard;", "@layer message-a, message-print-guard;"},
		{"sublayer", "@layer print-guard.x{}", "@layer message-print-guard.message-x{}"},
		{"anonymous layer", "@layer{p{color:red}}", "@layer{p{color:red}}"},
		{"layer in media", "@media print{@layer print-guard{p{color:red}}}", "@media print{@layer message-print-guard{p{color:red}}}"},
		{"comment in a layer name", "@layer/**/print-guard{}", "@layer message-print-guard{}"},
		{"removal in a layer prelude", "@layer @import url(x); print-guard{}", "@layer   message-print-guard{}"},
		{"names after the prelude", "@layer a{p{color:red}} b{color:blue}", "@layer message-a{p{color:red}} b{color:blue}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeCSS(tc.in, resolve); got != tc.want {
				t.Errorf("sanitizeCSS(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// A message's CSS must see a light page: the reader page is white in every
// appearance (pageStyle), but the engine reports prefers-color-scheme: dark
// when the desktop is dark, so a newsletter's dark @media rule would draw on
// the white page (frostyard/frostmail#269). A prefers-color-scheme feature
// answers light there, and a message cannot set the page's color-scheme.
func TestSanitizeCSSShowsALightPage(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"dark media rule", `@media (prefers-color-scheme: dark){body{background:#000}}`, `@media (grid:1){body{background:#000}}`},
		{"light media rule", `@media (prefers-color-scheme:light){p{color:#000}}`, `@media (grid:0){p{color:#000}}`},
		{"media rule with screen", `@media screen and (prefers-color-scheme: dark){p{color:#fff}}`, `@media screen and (grid:1){p{color:#fff}}`},
		{"negated media rule", `@media not all and (prefers-color-scheme: dark){p{color:#000}}`, `@media not all and (grid:1){p{color:#000}}`},
		{"media rule list", `@media (prefers-color-scheme:dark),(prefers-color-scheme:light){p{color:#000}}`, `@media (grid:1),(grid:0){p{color:#000}}`},
		{"escaped feature name", `@media (prefers-color-sch\65me: dark){p{color:#fff}}`, `@media (grid:1){p{color:#fff}}`},
		{"uppercase feature name and value", `@media (PREFERS-COLOR-SCHEME: DARK){p{color:#fff}}`, `@media (grid:1){p{color:#fff}}`},
		{"unknown value", `@media (prefers-color-scheme: no-preference){p{color:#fff}}`, `@media (grid:1){p{color:#fff}}`},
		{"comment around the colon", `@media (prefers-color-scheme/**/:/**/dark){p{color:#fff}}`, `@media (grid:1){p{color:#fff}}`},
		{"boolean form is kept", `@media (prefers-color-scheme){p{color:#fff}}`, `@media (prefers-color-scheme){p{color:#fff}}`},
		{"class name outside a media prelude", `.prefers-color-scheme{color:red}`, `.prefers-color-scheme{color:red}`},
		{"root color scheme", `:root{color-scheme:light dark}`, `:root{x-blocked:light dark}`},
		{"escaped color scheme property", `p{color-sch\65me:dark}`, `p{x-blocked:dark}`},
		{"uppercase color scheme property", `p{COLOR-SCHEME: DARK}`, `p{x-blocked: DARK}`},
		{"style attribute color scheme", `color-scheme: dark; color: red`, `x-blocked: dark; color: red`},
		{"color scheme selector", `.color-scheme{color:red}`, `.color-scheme{color:red}`},
		{"color scheme in a compound selector", `div.color-scheme:hover{color:red}`, `div.color-scheme:hover{color:red}`},
		{"light dark function is kept", `p{color:light-dark(#000,#fff)}`, `p{color:light-dark(#000,#fff)}`},
		{"color scheme after another declaration", `p{margin:0;color-scheme:dark}`, `p{margin:0;x-blocked:dark}`},
		{"color scheme after a nested rule", `div{span{color:red} color-scheme:dark}`, `div{span{color:red} x-blocked:dark}`},
		{"color scheme value is kept", `p{background-color-scheme:dark}`, `p{background-color-scheme:dark}`},
		// A removal writes only a space, so the declaration start before it
		// must survive: else a later color-scheme escapes the guard.
		{"color scheme after a removed at-rule", `:root{@page{}color-scheme:dark}`, `:root{ x-blocked:dark}`},
		{"color scheme after an import", `p{margin:0;@import url(x);color-scheme:dark}`, `p{margin:0; x-blocked:dark}`},
		{"color scheme after cdo", `:root{<!--color-scheme:dark}`, `:root{ x-blocked:dark}`},
		{"cdo after cdc at the start", `-->color-scheme:dark`, ` x-blocked:dark`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeCSS(tc.in, func(string) string { return "" }); got != tc.want {
				t.Errorf("sanitizeCSS(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCSSUnescape(t *testing.T) {
	cases := map[string]string{
		`url`:        "url",
		`\75 rl`:     "url",
		`\000075rl`:  "url",
		`beh\61vior`: "behavior",
		`a\:b`:       "a:b",
		`\0 x`:       "�x",
		`end\`:       `end\`,
	}
	for in, want := range cases {
		if got := cssUnescape(in); got != want {
			t.Errorf("cssUnescape(%q) = %q, want %q", in, got, want)
		}
	}
}

// A message's CSS cannot reach the page's own layers or its paper. The
// desktop app's print guard is the page's first layer: a rule in it, or
// in a layer within it, would beat the guard's own (M46). An @page rule
// prints text in the margins, outside the page's content (issue #238).
// However a message spells the name, what is left names no layer of the
// page and holds no @page rule.
func TestSanitizeCSSKeepsMessagesOffThePage(t *testing.T) {
	const hide = `{@media print{body>.print-only:first-child{display:none!important}}}`
	for _, in := range []string{
		"@layer print-guard" + hide,
		"@layer print-guard.x" + hide,
		"@layer x, print-guard;@layer print-guard" + hide,
		`@layer \70 rint-guard` + hide,
		`@layer \000070rint-guard` + hide,
		`@layer pr\69nt-guard` + hide,
		`@l\61yer print-guard` + hide,
		"@LAYER print-guard" + hide,
		"@layer /**/print-guard" + hide,
		"@layer @font-face{x:y} print-guard" + hide,
		"@layer url(x) print-guard" + hide,
		"@media print{@layer print-guard" + hide + "}",
		`@page{@top-left{content:"From: Bank"}}`,
		`@PAGE :first{margin:0}`,
		`@pa\67 e{size:1in}`,
		`@media print{@page{margin:-1in}}`,
	} {
		t.Run(in, func(t *testing.T) {
			out := sanitizeCSS(in, func(string) string { return "" })
			checkCSS(t, out, out)
		})
	}
}
