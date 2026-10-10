package davx

import (
	"net/url"
	"testing"
)

// TestSameOrigin: iCloud writes its home set with an explicit :443 and
// other hrefs without it; a default port is the same origin.
func TestSameOrigin(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"https://p37-caldav.icloud.com:443/1/calendars/", "https://p37-caldav.icloud.com/1/calendars/inbox/", true},
		{"https://P37-CalDAV.icloud.com/x", "https://p37-caldav.icloud.com:443/y", true},
		{"http://dav.example:80/", "http://dav.example/", true},
		{"https://dav.example:8443/", "https://dav.example/", false},
		{"http://dav.example/", "https://dav.example/", false},
		{"https://dav.example/", "https://evil.example/", false},
	} {
		a, _ := url.Parse(c.a)
		b, _ := url.Parse(c.b)
		if got := sameOrigin(a, b); got != c.want {
			t.Errorf("sameOrigin(%s, %s) = %v", c.a, c.b, got)
		}
	}
	base, _ := url.Parse("https://p37-caldav.icloud.com:443/1/calendars/")
	if path, ok := hrefPath(base, "https://p37-caldav.icloud.com/1/calendars/inbox/"); !ok || path != "/1/calendars/inbox/" {
		t.Errorf("hrefPath = %q, %v", path, ok)
	}
}
