package mcpserver

import "testing"

// TestMessageURL: a Message-ID percent-encoded as one segment, as the web
// app's encodeURIComponent does (app/src/lodge/routes.ts, permalink); none
// for no Message-ID, or for one the route would read as maild's ID.
func TestMessageURL(t *testing.T) {
	for id, want := range map[string]string{
		"gen-1-200@mailgen.test": "/mail/message/gen-1-200%40mailgen.test",
		"a/b%c+d@x.test":         "/mail/message/a%2Fb%25c%2Bd%40x.test",
		"ü!~*'()_.-@x":           "/mail/message/%C3%BC!~*'()_.-%40x",
		"a b?c#d&e=f;g:h,i$j<k>": "/mail/message/a%20b%3Fc%23d%26e%3Df%3Bg%3Ah%2Ci%24j%3Ck%3E",
		"012":                    "/mail/message/012",
		"12345678901234567":      "/mail/message/12345678901234567",
		"12345":                  "",
		"1234567890123456":       "",
		"":                       "",
		"  ":                     "",
	} {
		if got := messageURL(id); got != want {
			t.Errorf("messageURL(%q) = %q, want %q", id, got, want)
		}
	}
	if h, u := messageLink(""); h != "" || u != "" {
		t.Errorf("messageLink of none = %q, %q", h, u)
	}
	if h, u := messageLink("12345"); h != "12345" || u != "" {
		t.Errorf("messageLink(12345) = %q, %q; want the header and no URL", h, u)
	}
}
