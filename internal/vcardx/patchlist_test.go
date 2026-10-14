package vcardx

// CONTRACT TEST for task card T-0133 (docs/tasks). Do not edit.
// It uses patch_test.go's helpers (T-0132).

import (
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/contentline"
)

func labeled(pairs ...string) *[]Labeled {
	out := []Labeled{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Labeled{Label: pairs[i], Value: pairs[i+1]})
	}
	return &out
}

// readsBack checks that the patched card's lists are the ones given, as
// labels and values.
func readsBack(t *testing.T, out string, f Fields) {
	t.Helper()
	card, err := Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	same := func(a, b Labeled) bool { return a.Label == b.Label && a.Value == b.Value }
	for _, l := range []struct {
		name      string
		got, want *[]Labeled
	}{{"emails", &card.Emails, f.Emails}, {"phones", &card.Phones, f.Phones}, {"urls", &card.URLs, f.URLs}} {
		if l.want != nil && !slices.EqualFunc(*l.got, *l.want, same) {
			t.Errorf("%s read back as %+v, want %+v", l.name, *l.got, *l.want)
		}
	}
	if f.Addresses != nil && !slices.Equal(card.Addresses, *f.Addresses) {
		t.Errorf("addresses read back as %+v, want %+v", card.Addresses, *f.Addresses)
	}
}

func TestPatchListValueKeepsItsLine(t *testing.T) {
	src := fixture(t, "google.vcf")
	f := Fields{Emails: labeled("home", "ada@lovelace.example", "work", "Ada@Analytical.example", "Club", "ada@club.example")}
	got := patched(t, src, f)
	expect(t, got, strings.Replace(src, "TYPE=HOME:ada@example.com\r\n", "TYPE=HOME:ada@lovelace.example\r\n", 1))
	readsBack(t, got, f)
}

func TestPatchRelabels(t *testing.T) {
	src := fixture(t, "google.vcf")
	f := Fields{
		Emails: labeled("home", "ada@example.com", "other", "Ada@Analytical.example", "Club", "ada@club.example"),
		Phones: labeled("mobile", "+44 20 7946 0000", "mobile", "+44 20 7946 0009"),
	}
	got := patched(t, src, f)
	expect(t, got, strings.NewReplacer(
		"EMAIL;TYPE=INTERNET;TYPE=WORK:Ada@Analytical.example\r\n", "EMAIL;TYPE=INTERNET;TYPE=OTHER:Ada@Analytical.example\r\n",
		"TEL;TYPE=WORK;TYPE=FAX:+44 20 7946 0009\r\n", "TEL;TYPE=CELL:+44 20 7946 0009\r\n",
	).Replace(src))
	readsBack(t, got, f)
}

// Apple's labels live in groups: a custom label rewrites the group's
// X-ABLabel and leaves its line alone; a known one becomes a TYPE and the
// group label goes.
func TestPatchKeepsAppleGroups(t *testing.T) {
	src := fixture(t, "icloud.vcf")
	f := Fields{
		Emails: labeled("work", "grace@navy.example", "Lab", "grace@cobol.example"),
		Phones: labeled("mobile", "(555) 010-0199", "work", "(555) 010-0100", "Lab", "(555) 010-0111",
			"mobile", "(555) 010-0112"),
		Addresses: &[]Address{{Label: "work", Street: "1 Navy Way\nBuilding 2", Locality: "Alexandria", Region: "VA",
			Postcode: "22202", Country: "USA"}},
	}
	got := patched(t, src, f)
	expect(t, got, strings.NewReplacer(
		"item1.X-ABLabel:_$!<Other>!$_\r\n", "item1.X-ABLabel:Lab\r\n",
		"item3.TEL:(555) 010-0112\r\nitem3.X-ABLabel:_$!<WorkFAX>!$_\r\n", "item3.TEL;TYPE=CELL:(555) 010-0112\r\n",
		"item4.ADR;type=WORK;type=pref:;;1 Navy Way\\nBuilding 2;Arlington;VA;22202;USA\r\n",
		// A rewritten line is folded at 75 octets.
		contentline.Fold("item4.ADR;TYPE=WORK;TYPE=pref:;;1 Navy Way\\nBuilding 2;Alexandria;VA;22202;USA", "\r\n"),
	).Replace(src))
	readsBack(t, got, f)
	if !strings.Contains(got, "item4.X-ABADR:us\r\n") {
		t.Error("the address lost its group's X-ABADR")
	}
}

func TestPatchAddsAndRemoves(t *testing.T) {
	src := fixture(t, "google.vcf")
	f := Fields{
		Emails: labeled("home", "ada@example.com", "Chess", "ada@chess.example"),
		Phones: labeled("mobile", "+44 20 7946 0000", "fax", "+44 20 7946 0009", "pager", "+44 20 7946 0123"),
		Addresses: &[]Address{
			{Label: "home", Street: "12 St James's Square", Locality: "London", Postcode: "SW1Y 4JH", Country: "United Kingdom"},
			{Label: "work", Street: "1 Lab Way", Locality: "Cambridge", Postcode: "CB2 1TN", Country: "United Kingdom"},
		},
		URLs: labeled("", "https://ada.example/", "homepage", "https://lovelace.example/"),
	}
	got := patched(t, src, f)
	expect(t, got, strings.NewReplacer(
		"EMAIL;TYPE=INTERNET;TYPE=WORK:Ada@Analytical.example\r\nitem1.EMAIL;TYPE=INTERNET:ada@club.example\r\nitem1.X-ABLabel:Club\r\n",
		"item2.EMAIL;TYPE=INTERNET:ada@chess.example\r\nitem2.X-ABLabel:Chess\r\n",
		"TEL;TYPE=WORK;TYPE=FAX:+44 20 7946 0009\r\n", "TEL;TYPE=WORK;TYPE=FAX:+44 20 7946 0009\r\nTEL;TYPE=PAGER:+44 20 7946 0123\r\n",
		"United Kingdom\r\n", "United Kingdom\r\nADR;TYPE=WORK:;;1 Lab Way;Cambridge;;CB2 1TN;United Kingdom\r\n",
		"URL:https://ada.example/\r\n", "URL:https://ada.example/\r\nitem3.URL:https://lovelace.example/\r\nitem3.X-ABLabel:_$!<HomePage>!$_\r\n",
	).Replace(src))
	readsBack(t, got, f)
}

func TestPatchVersion4(t *testing.T) {
	src := fixture(t, "nextcloud.vcf")
	f := Fields{
		Emails: labeled("work", "alan@enigma.example", "home", "alan@home.example", "home", "alan@turing.example"),
		Phones: labeled("work", "+44 20 7946 1111", "home", "+44 20 7946 0002"),
	}
	got := patched(t, src, f)
	expect(t, got, strings.NewReplacer(
		"EMAIL;TYPE=work;PREF=1:mailto:alan@bletchley.example\r\n", "EMAIL;TYPE=work;PREF=1:mailto:alan@enigma.example\r\n",
		"EMAIL;TYPE=home:alan@home.example\r\n", "EMAIL;TYPE=home:alan@home.example\r\nEMAIL;TYPE=home:alan@turing.example\r\n",
		`TEL;VALUE=uri;TYPE="voice,cell":tel:+44-20-7946-0001`+"\r\n", "TEL;VALUE=uri;TYPE=voice;TYPE=work:tel:+44 20 7946 1111\r\n",
	).Replace(src))
	readsBack(t, got, f)
}

func TestPatchRemovesAList(t *testing.T) {
	src := fixture(t, "icloud.vcf")
	got := patched(t, src, Fields{Emails: &[]Labeled{}})
	expect(t, got, strings.NewReplacer(
		"EMAIL;type=INTERNET;type=WORK;type=pref:grace@navy.example\r\n", "",
		"item1.EMAIL;type=INTERNET:grace@cobol.example\r\n", "",
		"item1.X-ABLabel:_$!<Other>!$_\r\n", "",
	).Replace(src))
}

func TestNewCard(t *testing.T) {
	f := Fields{
		GivenName: ptr("Ada"), FamilyName: ptr("Lovelace"), Nickname: ptr("Ada"),
		Organization: ptr("Analytical Engines"), Title: ptr("Programmer"),
		Emails:    labeled("home", "ada@example.com", "Club", "ada@club.example"),
		Phones:    labeled("mobile", "+44 20 7946 0000"),
		Addresses: &[]Address{{Label: "home", Street: "12 St James's Square", Locality: "London", Postcode: "SW1Y 4JH", Country: "United Kingdom"}},
		URLs:      labeled("homepage", "https://ada.example/"),
		Birthday:  ptr("1815-12-10"),
		Note:      ptr("Met at the\nExhibition"),
	}
	out, err := NewCard("new-1", f, patchNow)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, string(out), cardLines(
		"BEGIN:VCARD", "VERSION:3.0", "PRODID:-//Frostyard//Frostmail//EN", "UID:new-1",
		"FN:Ada Lovelace", "N:Lovelace;Ada;;;", "NICKNAME:Ada", "ORG:Analytical Engines", "TITLE:Programmer",
		"EMAIL;TYPE=INTERNET;TYPE=HOME:ada@example.com",
		"item1.EMAIL;TYPE=INTERNET:ada@club.example", "item1.X-ABLabel:Club",
		"TEL;TYPE=CELL:+44 20 7946 0000",
		"ADR;TYPE=HOME:;;12 St James's Square;London;;SW1Y 4JH;United Kingdom",
		"item2.URL:https://ada.example/", "item2.X-ABLabel:_$!<HomePage>!$_",
		"BDAY:1815-12-10", `NOTE:Met at the\nExhibition`, "REV:2026-10-14T15:30:00Z", "END:VCARD",
	))
	readsBack(t, string(out), f)

	out, err = NewCard("new-2", Fields{Emails: labeled("", "x@y.test")}, patchNow)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, string(out), cardLines("BEGIN:VCARD", "VERSION:3.0", "PRODID:-//Frostyard//Frostmail//EN", "UID:new-2",
		"FN:x@y.test", "N:;;;;", "EMAIL;TYPE=INTERNET:x@y.test", "REV:2026-10-14T15:30:00Z", "END:VCARD"))

	if _, err := NewCard("new-3", Fields{Birthday: ptr("1815-13-10")}, patchNow); err == nil {
		t.Error("NewCard took an impossible birthday")
	}
}
