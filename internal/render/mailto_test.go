package render

import (
	"encoding/json/v2"
	"html"
	"os"
	"testing"
)

// These cases come from frostui v0.4.0's mailtoCases and also exercise the
// web app's openLink. Test the HTML sanitizer, since lodge-content opens
// links directly without the app's click handler.
func TestSanitizeMailtoFields(t *testing.T) {
	data, err := os.ReadFile("testdata/mailto.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Input, Want string }
	if err := json.Unmarshal(data, &cases, json.MatchCaseInsensitiveNames(true)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var r testResolver
			input := `<a href="` + html.EscapeString(tc.Input) + `">link</a>`
			want := `<a>link</a>`
			if tc.Want != "" {
				want = `<a href="` + html.EscapeString(tc.Want) + `">link</a>`
			}
			got, err := sanitizeHTML(input, r.resolve)
			if err != nil || got != want {
				t.Fatalf("sanitize = %q, %v; want %q", got, err, want)
			}
			again, err := sanitizeHTML(got, r.resolve)
			if err != nil || again != got {
				t.Fatalf("second sanitize = %q, %v; want %q", again, err, got)
			}
		})
	}
}
