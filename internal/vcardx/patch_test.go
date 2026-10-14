package vcardx

// CONTRACT TEST for task card T-0132 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

var patchNow = time.Date(2026, 10, 14, 15, 30, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func cardLines(lines ...string) string { return strings.Join(lines, "\r\n") + "\r\n" }

func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func patched(t *testing.T, src string, f Fields) string {
	t.Helper()
	out, err := Patch([]byte(src), f, patchNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out); err != nil {
		t.Fatalf("the patched card does not parse: %v\n%s", err, out)
	}
	return string(out)
}

func expect(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("patched:\n%s\nwant:\n%s", got, want)
	}
}

func TestPatchNamesKeepTheRest(t *testing.T) {
	src := fixture(t, "icloud.vcf")
	got := patched(t, src, Fields{GivenName: ptr("Grace B.")})
	expect(t, got, strings.NewReplacer(
		"N:Hopper;Grace;Brewster Murray;Rear Admiral;\r\n", "N:Hopper;Grace B.;Brewster Murray;Rear Admiral;\r\n",
		"FN:Grace Hopper\r\n", "FN:Grace B. Hopper\r\n",
	).Replace(src))
}

func TestPatchKeepsAFormattedNameTheUserChose(t *testing.T) {
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "N:Hopper;Grace;;;", "FN:Admiral Hopper", "UID:a1", "END:VCARD")
	got := patched(t, src, Fields{FamilyName: ptr("Murray")})
	expect(t, got, strings.Replace(src, "N:Hopper;Grace;;;", "N:Murray;Grace;;;", 1))
}

func TestPatchWritesAMissingFormattedName(t *testing.T) {
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "N:;;;;", "UID:a2", "END:VCARD")
	got := patched(t, src, Fields{GivenName: ptr("Ada"), FamilyName: ptr("Lovelace")})
	expect(t, got, cardLines("BEGIN:VCARD", "VERSION:3.0", "N:Lovelace;Ada;;;", "UID:a2", "FN:Ada Lovelace", "END:VCARD"))
}

func TestPatchCompanyCard(t *testing.T) {
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "N:;;;;", "FN:Acme Corp", "ORG:Acme Corp;Sales",
		"X-ABShowAs:COMPANY", "UID:c1", "END:VCARD")
	got := patched(t, src, Fields{Organization: ptr("Acme, Inc.")})
	expect(t, got, strings.NewReplacer(
		"FN:Acme Corp\r\n", "FN:Acme\\, Inc.\r\n",
		"ORG:Acme Corp;Sales\r\n", "ORG:Acme\\, Inc.;Sales\r\n",
	).Replace(src))
}

func TestPatchTextFields(t *testing.T) {
	src := fixture(t, "google.vcf")
	got := patched(t, src, Fields{
		Nickname: ptr("Countess"),
		Title:    ptr(""),
		Note:     ptr("Met at the\nExhibition, 1851; again 1852"),
	})
	expect(t, got, strings.NewReplacer(
		"TITLE:Programmer\r\n", "",
		"NOTE:Met at the\\nExhibition\\, 1851\r\n", "NOTE:Met at the\\nExhibition\\, 1851\\; again 1852\r\n",
		"END:VCARD\r\n", "NICKNAME:Countess\r\nEND:VCARD\r\n",
	).Replace(src))
	card, err := Parse([]byte(got))
	if err != nil || card.Nickname != "Countess" || card.Title != "" || card.Note != "Met at the\nExhibition, 1851; again 1852" {
		t.Errorf("read back: %+v, %v", card, err)
	}
}

// Inserted lines come in a fixed order, before END:VCARD.
func TestPatchInsertsInOrder(t *testing.T) {
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann", "UID:a3", "END:VCARD")
	got := patched(t, src, Fields{Note: ptr("n"), Title: ptr("Boss"), Nickname: ptr("Annie"), Birthday: ptr("1990-05-01")})
	expect(t, got, cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann", "UID:a3",
		"NICKNAME:Annie", "TITLE:Boss", "BDAY:1990-05-01", "NOTE:n", "END:VCARD"))
}

func TestPatchKeepsGroupsAndParametersAndDropsRepeats(t *testing.T) {
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann", "item9.TITLE:Boss", "NOTE;LANGUAGE=en:old",
		"X-CUSTOM:keep me", "NOTE:older", "UID:a4", "END:VCARD")
	got := patched(t, src, Fields{Title: ptr("Chief"), Note: ptr("new")})
	expect(t, got, cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann", "item9.TITLE:Chief", "NOTE;LANGUAGE=en:new",
		"X-CUSTOM:keep me", "UID:a4", "END:VCARD"))
}

func TestPatchBirthday(t *testing.T) {
	google := fixture(t, "google.vcf")
	icloud := fixture(t, "icloud.vcf")
	nextcloud := fixture(t, "nextcloud.vcf")
	for _, tc := range []struct {
		name, src, birthday, line, want string
	}{
		{"v3 without a year", google, "--12-10", "BDAY:1815-12-10\r\n", "BDAY:--12-10\r\n"},
		{"v3 with a year", google, "1816-01-02", "BDAY:1815-12-10\r\n", "BDAY:1816-01-02\r\n"},
		{"v3 removed", google, "", "BDAY:1815-12-10\r\n", ""},
		{"Apple's year it omits", icloud, "--12-25", "BDAY;X-APPLE-OMIT-YEAR=1604:1604-12-09\r\n",
			"BDAY;X-APPLE-OMIT-YEAR=1604:1604-12-25\r\n"},
		{"Apple's card given a year", icloud, "1906-12-09", "BDAY;X-APPLE-OMIT-YEAR=1604:1604-12-09\r\n",
			"BDAY:1906-12-09\r\n"},
		{"v4 with a year", nextcloud, "1912-06-23", "BDAY:--0623\r\n", "BDAY:19120623\r\n"},
		{"v4 without a year", nextcloud, "--06-24", "BDAY:--0623\r\n", "BDAY:--0624\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := patched(t, tc.src, Fields{Birthday: ptr(tc.birthday)})
			expect(t, got, strings.Replace(tc.src, tc.line, tc.want, 1))
			card, _ := Parse([]byte(got))
			if card.Birthday != tc.birthday {
				t.Errorf("read back birthday %q, want %q", card.Birthday, tc.birthday)
			}
		})
	}
	for _, bad := range []string{"12/09/1906", "1906-02-30", "--13-01", "1906-12"} {
		if _, err := Patch([]byte(google), Fields{Birthday: ptr(bad)}, patchNow); err == nil {
			t.Errorf("birthday %q was accepted", bad)
		}
	}
}

func TestPatchPhoto(t *testing.T) {
	jpeg := bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3}, 20)
	png := []byte("\x89PNG\r\n\x1a\nfrostmail")
	google := fixture(t, "google.vcf")
	got := patched(t, google, Fields{Photo: &Photo{Type: "image/jpeg", Data: jpeg}})
	line := contentline.Fold("PHOTO;ENCODING=b;TYPE=JPEG:"+base64.StdEncoding.EncodeToString(jpeg), "\r\n")
	expect(t, got, strings.Replace(google, "PHOTO:https://lh3.googleusercontent.com/contacts/AbC123\r\n", line, 1))
	if card, _ := Parse([]byte(got)); !bytes.Equal(card.Photo.Data, jpeg) || card.Photo.Type != "image/jpeg" {
		t.Errorf("read back photo %+v", card.Photo)
	}

	nextcloud := fixture(t, "nextcloud.vcf")
	got = patched(t, nextcloud, Fields{Photo: &Photo{Type: "image/png", Data: png}})
	old := "PHOTO:data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk\r\n" +
		" YPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg==\r\n"
	line = contentline.Fold("PHOTO:data:image/png;base64,"+base64.StdEncoding.EncodeToString(png), "\r\n")
	expect(t, got, strings.Replace(nextcloud, old, line, 1))

	icloud := fixture(t, "icloud.vcf")
	got = patched(t, icloud, Fields{Photo: &Photo{}})
	if strings.Contains(got, "PHOTO") || strings.Contains(got, "yQALCAAB") {
		t.Errorf("photo not removed:\n%s", got)
	}
	start := strings.Index(icloud, "PHOTO;")
	end := strings.Index(icloud, "UID:")
	expect(t, got, icloud[:start]+icloud[end:])

	if _, err := Patch([]byte(google), Fields{Photo: &Photo{Type: "image/gif", Data: []byte("GIF89a")}}, patchNow); err == nil {
		t.Error("a GIF was accepted")
	}
}

func TestPatchRevision(t *testing.T) {
	v3 := cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann", "REV:2020-01-01T00:00:00Z", "UID:a5", "END:VCARD")
	expect(t, patched(t, v3, Fields{Title: ptr("Boss")}), cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann",
		"REV:2026-10-14T15:30:00Z", "UID:a5", "TITLE:Boss", "END:VCARD"))
	v4 := cardLines("BEGIN:VCARD", "VERSION:4.0", "FN:Ann", "REV:20200101T000000Z", "UID:a6", "END:VCARD")
	expect(t, patched(t, v4, Fields{Title: ptr("Boss")}), cardLines("BEGIN:VCARD", "VERSION:4.0", "FN:Ann",
		"REV:20261014T153000Z", "UID:a6", "TITLE:Boss", "END:VCARD"))
	if got := patched(t, fixture(t, "google.vcf"), Fields{Title: ptr("Boss")}); strings.Contains(got, "REV") {
		t.Errorf("REV added to a card without one:\n%s", got)
	}
	// A change of nothing changes nothing, REV included.
	if got := patched(t, v3, Fields{}); got != v3 {
		t.Errorf("an empty change rewrote the card:\n%s", got)
	}
}

func TestPatchKeepsLineEndingsAndFolds(t *testing.T) {
	group := fixture(t, "group.vcf")
	got := patched(t, group, Fields{Nickname: ptr("Readers")})
	expect(t, got, strings.Replace(group, "END:VCARD\n", "NICKNAME:Readers\nEND:VCARD\n", 1))

	long := strings.Repeat("All work and no play. ", 8)
	src := cardLines("BEGIN:VCARD", "VERSION:3.0", "FN:Ann", "UID:a7", "END:VCARD")
	got = patched(t, src, Fields{Note: ptr(long)})
	want := strings.Replace(src, "END:VCARD\r\n", contentline.Fold("NOTE:"+long, "\r\n")+"END:VCARD\r\n", 1)
	expect(t, got, want)
	for line := range strings.SplitSeq(strings.TrimSuffix(got, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("line of %d octets: %q", len(line), line)
		}
	}
}

func TestPatchRefusesWhatIsNotACard(t *testing.T) {
	for _, src := range []string{"not a card", "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"} {
		if _, err := Patch([]byte(src), Fields{Title: ptr("x")}, patchNow); err == nil {
			t.Errorf("patched %q", src)
		}
	}
}
