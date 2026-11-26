package compose_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/compose"
)

func TestParseAddresses(t *testing.T) {
	good, bad := compose.ParseAddresses(`"Smith, Bob" <bob@x.test>; ann@x.test, ANN@x.test, nope`)
	if len(good) != 2 || good[0].Name != "Smith, Bob" || good[1].Address != "ann@x.test" {
		t.Errorf("good %+v", good)
	}
	if len(bad) != 1 || bad[0] != "nope" {
		t.Errorf("bad %q", bad)
	}
}

func TestSendChoices(t *testing.T) {
	morning := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	if got := compose.SendChoices(morning); len(got) != 2 || got[0].Label != "Send 9:00 PM Tonight" || got[1].Label != "Send 8:00 AM Tomorrow" {
		t.Errorf("morning: %+v", got)
	}
	late := time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC)
	if got := compose.SendChoices(late); len(got) != 1 {
		t.Errorf("late: %+v", got)
	}
}

// M26: what a sender's name or address says never makes another recipient.
func TestHostileAddressesStayOneRecipient(t *testing.T) {
	const real = "real@x.test"
	for _, name := range []string{
		"extra@evil.test\nVictim", "extra@evil.test\r\nVictim", "a, extra@evil.test", "a; extra@evil.test",
		`a" <extra@evil.test>, "b`, "<extra@evil.test>", "a <extra@evil.test> b", "\"", "extra@evil.test\u2028Victim", "tab\there", "\x00\x1f",
	} {
		a := api.Address{Name: name, Address: real}
		tok := compose.Token(a)
		good, bad := compose.ParseAddresses(tok)
		if len(good) != 1 || len(bad) != 0 || good[0].Address != real {
			t.Errorf("name %q: token %q reads as %+v, %q", name, tok, good, bad)
		}
		both, bad := compose.ParseAddresses(strings.Join([]string{tok, "bo@x.test"}, ","))
		if len(both) != 2 || len(bad) != 0 {
			t.Errorf("name %q beside another: %+v, %q", name, both, bad)
		}
	}
	// net/mail unquotes a local part, so an address can hold what splits.
	for _, address := range []string{"extra@evil.test,b@x.test", "extra@evil.test\nb@x.test", "a;b@x.test", `"a,b"@x.test`, "a b@x.test", "<extra@evil.test>"} {
		for _, name := range []string{"", "Name"} {
			tok := compose.Token(api.Address{Name: name, Address: address})
			if good, bad := compose.ParseAddresses(tok); len(good) != 0 || len(bad) != 1 {
				t.Errorf("address %q, name %q: token %q reads as %+v, %q", address, name, tok, good, bad)
			}
		}
	}
}

// Ordinary names read back as they were.
func TestTokensKeepOrdinaryNames(t *testing.T) {
	for _, name := range []string{"Ann Example", "Smith, Bob", "J. Smith", "Ann (work)"} {
		tok := compose.Token(api.Address{Name: name, Address: "a@x.test"})
		good, bad := compose.ParseAddresses(tok)
		if len(good) != 1 || len(bad) != 0 || good[0].Name != name {
			t.Errorf("name %q: token %q reads as %+v, %q", name, tok, good, bad)
		}
	}
}

// The cases of attachmentReminder.test.ts.
func TestMentionsAttachment(t *testing.T) {
	for text, want := range map[string]bool{
		"I attached the file": true, "see attachment.": true, "Enclosed is it": true, "ATTACHMENTS": true,
		"unattached": false, "detached": false, "attachés": false, "nothing here": false,
	} {
		if got := compose.MentionsAttachment(text); got != want {
			t.Errorf("%q = %v", text, got)
		}
	}
}

func TestOwnTextLeavesOutQuotesAttributionAndSignature(t *testing.T) {
	src := `<p>Hello</p><p>Regards Ann</p><p>On Monday, Bo wrote:</p><blockquote><p>I attached it</p></blockquote>`
	if got := compose.OwnText(src, `<p>Regards Ann</p>`); got != "Hello" {
		t.Errorf("own text = %q", got)
	}
}

func TestNeedsReminder(t *testing.T) {
	if !compose.NeedsReminder(`<p>I attached it</p>`, "", 0) {
		t.Error("no reminder for a mention with nothing attached")
	}
	if compose.NeedsReminder(`<p>I attached it</p>`, "", 1) {
		t.Error("a reminder with a chip")
	}
	if compose.NeedsReminder(`<p>I attached it</p><p><img src="data:x"></p>`, "", 0) {
		t.Error("a reminder with an image in the body")
	}
	if !compose.NeedsReminder(`<p>I attached it</p><blockquote><img src="data:x"></blockquote>`, "", 0) {
		t.Error("a quoted image counted as attached")
	}
}

func TestAllowedLink(t *testing.T) {
	for href, want := range map[string]bool{
		"https://example.org": true, " HTTP://example.org": true, "mailto:a@x.test": true,
		"javascript:alert(1)": false, "file:///etc/passwd": false, "data:text/html,x": false, "": false,
	} {
		if got := compose.AllowedLink(href); got != want {
			t.Errorf("AllowedLink(%q) = %v", href, got)
		}
	}
}
