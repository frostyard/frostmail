package engine

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEmlName(t *testing.T) {
	for subject, want := range map[string]string{
		"Lunch":          "Lunch.eml",
		"  Re: a/b\\c  ": "Re: a-b-c.eml",
		"":               "(no subject).eml",
		"..hidden":       "hidden.eml",
		"tab\there":      "tab-here.eml",
		"Résumé (final)": "Résumé (final).eml",
	} {
		if got := emlName(subject); got != want {
			t.Errorf("emlName(%q) = %q, want %q", subject, got, want)
		}
	}
	long := emlName(strings.Repeat("é", 150))
	if !utf8.ValidString(long) || len(long) > 200+len(".eml") || !strings.HasSuffix(long, ".eml") {
		t.Errorf("a long subject = %q (%d bytes), want valid UTF-8 within 200 bytes and .eml", long, len(long))
	}
}
