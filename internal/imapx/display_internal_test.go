package imapx

import (
	"bytes"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// Exchange's sections carry the CRLF of the boundary that follows them:
// the copy cuts a part to its BODYSTRUCTURE size when that is all there
// is more, and keeps a part's own line breaks.
func TestDisplayCopyCutsExchangesBoundaryCRLF(t *testing.T) {
	html := []byte("<p>Hi</p>")
	text := []byte("Hi.\r\n")
	bs := &imap.BodyStructureMultiPart{
		Subtype:  "alternative",
		Extended: &imap.BodyStructureMultiPartExt{Params: map[string]string{"boundary": "b"}},
		Children: []imap.BodyStructure{
			&imap.BodyStructureSinglePart{Type: "text", Subtype: "plain", Size: uint32(len(text))},
			&imap.BodyStructureSinglePart{Type: "text", Subtype: "html", Size: uint32(len(html))},
		},
	}
	plan := planDisplay(bs)
	sections := map[string][]byte{
		"1.MIME": []byte("Content-Type: text/plain\r\n\r\n"),
		"1":      text, // its own line break, within its size
		"2.MIME": []byte("Content-Type: text/html\r\n\r\n"),
		"2":      append(append([]byte(nil), html...), "\r\n"...), // Exchange's extra CRLF
	}
	found := func(sec *imap.FetchItemBodySection) []byte {
		key := pathKey(sec.Part)
		if sec.Specifier == imap.PartSpecifierMIME {
			key += ".MIME"
		}
		return sections[key]
	}
	var out bytes.Buffer
	if err := plan.write(&out, bs, nil, []byte("Content-Type: multipart/alternative; boundary=b\r\n\r\n"), found); err != nil {
		t.Fatal(err)
	}
	want := "Content-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain\r\n\r\nHi.\r\n" +
		"\r\n--b\r\nContent-Type: text/html\r\n\r\n<p>Hi</p>" +
		"\r\n--b--\r\n"
	if out.String() != want {
		t.Errorf("copy =\n%q\nwant\n%q", out.String(), want)
	}
}
