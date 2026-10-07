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
		{"import", "@import url(https://e.test/x.css); p{color:red}", " p{color:red}"},
		{"import string", `@import "https://e.test/x.css"; p{color:red}`, " p{color:red}"},
		{"font face", "@font-face{font-family:x;src:url(https://e.test/x.woff)} p{font-family:x}", " p{font-family:x}"},
		{"expression", "width:expression(alert(1)); color:red", "width:none; color:red"},
		{"image-set", `background-image:image-set("a.png" 1x)`, "background-image:none"},
		{"behavior", "behavior:url(x.htc)", "x-blocked:none"},
		{"escaped behavior", `beh\61vior:url(x.htc)`, "x-blocked:none"},
		{"moz binding", "-moz-binding:url(x.xml)", "x-blocked:none"},
		{"style breakout", `p{content:"</style><b>"}`, `p{content:"\3c /style>\3c b>"}`},
		{"comments and cdo", "<!-- p{color:red} /* x */ -->", " p{color:red}  "},
		{"unterminated url string", `\75rl("`, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeCSS(tc.in, resolve); got != tc.want {
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
