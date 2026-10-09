package vcardx

// CONTRACT TEST for task card T-0060 (docs/tasks). Do not edit.

import (
	"encoding/base64"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/contentline"
)

func parseFile(t *testing.T, name string) *Card {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return c
}

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func check(t *testing.T, name string, got, want *Card) {
	t.Helper()
	v1, v2 := reflect.ValueOf(*got), reflect.ValueOf(*want)
	for i := range v1.NumField() {
		f := v1.Type().Field(i).Name
		if !reflect.DeepEqual(v1.Field(i).Interface(), v2.Field(i).Interface()) {
			t.Errorf("%s: %s = %#v\nwant %#v", name, f, v1.Field(i).Interface(), v2.Field(i).Interface())
		}
	}
}

// TestGoogle: a contact as Google's CardDAV serves it (vCard 3.0, TYPE
// parameters, a grouped custom label, a photo URL).
func TestGoogle(t *testing.T) {
	check(t, "google", parseFile(t, "google.vcf"), &Card{
		UID:           "2b1f0a9c8e7d6c5b",
		FormattedName: "Ada Lovelace",
		FamilyName:    "Lovelace",
		GivenName:     "Ada",
		Organization:  "Analytical Engines Ltd.",
		Title:         "Programmer",
		Emails: []Labeled{
			{Label: "home", Value: "ada@example.com"},
			{Label: "work", Value: "Ada@Analytical.example"},
			{Label: "Club", Value: "ada@club.example"},
		},
		Phones: []Labeled{
			{Label: "mobile", Value: "+44 20 7946 0000"},
			{Label: "fax", Value: "+44 20 7946 0009"},
		},
		Addresses: []Address{{Label: "home", Street: "12 St James's Square", Locality: "London", Postcode: "SW1Y 4JH", Country: "United Kingdom"}},
		URLs:      []Labeled{{Value: "https://ada.example/"}},
		Birthday:  "1815-12-10",
		Note:      "Met at the\nExhibition, 1851",
		Photo:     Photo{URI: "https://lh3.googleusercontent.com/contacts/AbC123"},
	})
}

// TestICloud: a contact as iCloud serves it (Apple's item groups and
// X-ABLabel labels, pref types, an inline folded photo, a birthday without
// a year).
func TestICloud(t *testing.T) {
	photo := "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAMCAgICAgMCAgIDAwMDBAYEBAQEBAgGBgUGCQgKCgkICQkKDA8MCgsOCwkJDRENDg8QEBEQCgwSExIQEw8QEBD/" +
		"yQALCAABAAEBAREA/8wABgAQEAX/2gAIAQEAAD8A0s8g/9k="
	check(t, "icloud", parseFile(t, "icloud.vcf"), &Card{
		UID:           "6F3B9A2E-1C4D-4E5F-8A9B-0C1D2E3F4A5B",
		FormattedName: "Grace Hopper",
		FamilyName:    "Hopper",
		GivenName:     "Grace",
		MiddleName:    "Brewster Murray",
		Prefix:        "Rear Admiral",
		Nickname:      "Amazing Grace",
		Organization:  "United States Navy",
		Department:    "Bureau of Ordnance, Computation Project",
		Title:         "Computer Scientist",
		Emails: []Labeled{
			{Label: "work", Value: "grace@navy.example", Pref: true},
			{Label: "other", Value: "grace@cobol.example"},
		},
		Phones: []Labeled{
			{Label: "mobile", Value: "(555) 010-0199", Pref: true},
			{Label: "work", Value: "(555) 010-0100"},
			{Label: "Lab", Value: "(555) 010-0111"},
			{Label: "fax", Value: "(555) 010-0112"},
		},
		Addresses: []Address{{Label: "work", Street: "1 Navy Way\nBuilding 2", Locality: "Arlington", Region: "VA", Postcode: "22202", Country: "USA"}},
		URLs:      []Labeled{{Label: "homepage", Value: "https://grace.example", Pref: true}},
		Birthday:  "--12-09",
		Photo:     Photo{Type: "image/jpeg", Data: b64(t, photo)},
	})
}

// TestNextcloud: a vCard 4.0 (PREF=1, URI values, a quoted TYPE list, a
// data: photo, a birthday without a year).
func TestNextcloud(t *testing.T) {
	check(t, "nextcloud", parseFile(t, "nextcloud.vcf"), &Card{
		UID:           "urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b1",
		Kind:          "individual",
		FormattedName: "Alan Turing",
		FamilyName:    "Turing",
		GivenName:     "Alan",
		MiddleName:    "Mathison",
		Suffix:        "OBE",
		Emails: []Labeled{
			{Label: "work", Value: "alan@bletchley.example", Pref: true},
			{Label: "home", Value: "alan@home.example"},
		},
		Phones: []Labeled{
			{Label: "mobile", Value: "+44-20-7946-0001"},
			{Label: "home", Value: "+44 20 7946 0002"},
		},
		Addresses: []Address{{Label: "home", Street: "Adlington Road", Locality: "Wilmslow", Region: "Cheshire", Postcode: "SK9 4AS", Country: "UK"}},
		Birthday:  "--06-23",
		Note:      "Line one\nLine two",
		Photo: Photo{Type: "image/png", Data: b64(t,
			"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg==")},
	})
}

func TestGroup(t *testing.T) {
	c := parseFile(t, "group.vcf")
	if c.Kind != "group" || c.FormattedName != "Book Club" || c.UID != "group-1" {
		t.Errorf("group = %+v", c)
	}
}

func card(lines ...string) []byte {
	return []byte("BEGIN:VCARD\r\nVERSION:3.0\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCARD\r\n")
}

func TestNames(t *testing.T) {
	for _, tc := range []struct {
		lines         []string
		display, sort string
	}{
		{[]string{"FN:Ada Lovelace", "N:Lovelace;Ada;;;"}, "Ada Lovelace", "lovelace ada"},
		{[]string{"FN:  ", "N:Lovelace;Ada;;;"}, "Ada Lovelace", "lovelace ada"},
		{[]string{"N:;Ada;;;"}, "Ada", "ada"},
		{[]string{"N:Lovelace;;;;"}, "Lovelace", "lovelace"},
		{[]string{"NICKNAME:Countess"}, "Countess", "countess"},
		{[]string{"ORG:Analytical Engines Ltd.;R&D"}, "Analytical Engines Ltd.", "analytical engines ltd."},
		{[]string{"EMAIL:ada@example.com"}, "ada@example.com", "ada@example.com"},
		{[]string{"UID:x"}, "", ""},
		{[]string{"FN:Émile Zola", "N:Zola;Émile;;;"}, "Émile Zola", "zola émile"},
	} {
		c, err := Parse(card(tc.lines...))
		if err != nil {
			t.Fatalf("%q: %v", tc.lines, err)
		}
		if got := c.DisplayName(); got != tc.display {
			t.Errorf("%q: DisplayName = %q, want %q", tc.lines, got, tc.display)
		}
		if got := c.SortKey(); got != tc.sort {
			t.Errorf("%q: SortKey = %q, want %q", tc.lines, got, tc.sort)
		}
	}
}

func TestDetails(t *testing.T) {
	for _, tc := range []struct {
		line string
		get  func(*Card) any
		want any
	}{
		{"BDAY:19800401", func(c *Card) any { return c.Birthday }, "1980-04-01"},
		{"BDAY:1980-04-01T00:00:00Z", func(c *Card) any { return c.Birthday }, "1980-04-01"},
		{"BDAY:--04-01", func(c *Card) any { return c.Birthday }, "--04-01"},
		{"BDAY:April first", func(c *Card) any { return c.Birthday }, ""},
		{"EMAIL;TYPE=INTERNET:  MAILTO:a@b.example ", func(c *Card) any { return c.Emails }, []Labeled{{Value: "a@b.example"}}},
		{"EMAIL;TYPE=OTHER;TYPE=WORK:a@b.example", func(c *Card) any { return c.Emails }, []Labeled{{Label: "work", Value: "a@b.example"}}},
		{"TEL;TYPE=IPHONE:1", func(c *Card) any { return c.Phones }, []Labeled{{Label: "mobile", Value: "1"}}},
		{"TEL;TYPE=PAGER:1", func(c *Card) any { return c.Phones }, []Labeled{{Label: "pager", Value: "1"}}},
		{"TEL;TYPE=MAIN:1", func(c *Card) any { return c.Phones }, []Labeled{{Label: "main", Value: "1"}}},
		{"TEL;TYPE=VOICE:1", func(c *Card) any { return c.Phones }, []Labeled{{Value: "1"}}},
		{"ADR:PO Box 7;Suite 3;1 Main St;Town;;;", func(c *Card) any { return c.Addresses },
			[]Address{{Street: "PO Box 7\nSuite 3\n1 Main St", Locality: "Town"}}},
		{"PHOTO;VALUE=uri:http://example.com/p.jpg", func(c *Card) any { return c.Photo }, Photo{URI: "http://example.com/p.jpg"}},
		{"PHOTO;ENCODING=BASE64:not base64!", func(c *Card) any { return c.Photo }, Photo{}},
		{"PHOTO;ENCODING=b:iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ", func(c *Card) any { return c.Photo.Type }, "image/png"},
		{"PHOTO;ENCODING=b;TYPE=image/gif:R0lGODlhAQABAAAAACw=", func(c *Card) any { return c.Photo.Type }, "image/gif"},
		{"X-ADDRESSBOOKSERVER-KIND:Group", func(c *Card) any { return c.Kind }, "group"},
	} {
		c, err := Parse(card(tc.line))
		if err != nil {
			t.Fatalf("%q: %v", tc.line, err)
		}
		if got := tc.get(c); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %#v, want %#v", tc.line, got, tc.want)
		}
	}
}

func TestLFAndLowercase(t *testing.T) {
	c, err := Parse([]byte("begin:vcard\nversion:3.0\nfn:Lower Case\nemail;type=home:l@c.example\nend:vcard\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.FormattedName != "Lower Case" || len(c.Emails) != 1 || c.Emails[0].Label != "home" {
		t.Errorf("card = %+v", c)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")); err == nil {
		t.Error("a calendar parsed as a vCard")
	}
	if _, err := Parse([]byte("BEGIN:VCARD\r\nFN:open")); !errors.Is(err, contentline.ErrSyntax) {
		t.Errorf("unclosed vCard: %v, want contentline.ErrSyntax", err)
	}
}

func TestNew(t *testing.T) {
	raw := New("uid-1", "Jane Q. Doe", "jane@example.com")
	want := "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Frostyard//Frostmail//EN\r\nUID:uid-1\r\n" +
		"FN:Jane Q. Doe\r\nN:Doe;Jane Q.;;;\r\nEMAIL;TYPE=INTERNET:jane@example.com\r\nEND:VCARD\r\n"
	if string(raw) != want {
		t.Errorf("New =\n%q\nwant\n%q", raw, want)
	}
	for _, tc := range []struct{ name, fn, n string }{
		{"Cher", "FN:Cher", "N:;Cher;;;"},
		{"", "FN:x@example.com", "N:;;;;"},
		{"  Doe,  Jane; Jr  ", `FN:Doe\,  Jane\; Jr`, `N:Jr;Doe\, Jane\;;;;`},
	} {
		raw := string(New("u", tc.name, "x@example.com"))
		if !strings.Contains(raw, "\r\n"+tc.fn+"\r\n") || !strings.Contains(raw, "\r\n"+tc.n+"\r\n") {
			t.Errorf("New(%q) =\n%s\nwant %s and %s", tc.name, raw, tc.fn, tc.n)
		}
	}
	long := New("u", strings.Repeat("Bartholomew ", 10)+"Smith", "x@example.com")
	for _, l := range strings.Split(string(long), "\r\n") {
		if len(l) > 75 {
			t.Errorf("unfolded line %q", l)
		}
	}
	c, err := Parse(long)
	if err != nil || c.FamilyName != "Smith" || c.Emails[0].Value != "x@example.com" {
		t.Errorf("Parse(New) = %+v, %v", c, err)
	}
}
