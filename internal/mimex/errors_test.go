package mimex

import (
	"errors"
	"io"
	"testing"
)

// unparsable is a message whose first header line has no colon: go-message
// cannot read it at all.
const unparsable = "no colon here\r\n\r\nbody\r\n"

const unparsableText = "mimex: read message: message: malformed MIME header line: no colon here\r\n"

// A message that cannot be parsed gives an error that matches ErrParse, with
// its text as it was; a callback's own error does not match it.
func TestParseErrorsMatchErrParse(t *testing.T) {
	raw := []byte(unparsable)
	_, _, textErr := BodyText(raw)
	_, htmlErr := BodyHTML(raw)
	walkErr := WalkParts(raw, func(PartInfo, io.Reader) error { return nil })
	for name, err := range map[string]error{"BodyText": textErr, "BodyHTML": htmlErr, "WalkParts": walkErr} {
		if err == nil || err.Error() != unparsableText || !errors.Is(err, ErrParse) {
			t.Errorf("%s = %v; want %q matching ErrParse", name, err, unparsableText)
		}
	}

	// A broken part inside a multipart: the walk stops on it.
	nested := []byte("Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nbad header line\r\n\r\nx\r\n--b--\r\n")
	if err := WalkParts(nested, func(PartInfo, io.Reader) error { return nil }); err == nil || !errors.Is(err, ErrParse) {
		t.Errorf("WalkParts of a broken part = %v; want an error matching ErrParse", err)
	}

	stop := errors.New("stop")
	ok := []byte("Subject: x\r\n\r\nbody\r\n")
	if err := WalkParts(ok, func(PartInfo, io.Reader) error { return stop }); !errors.Is(err, stop) || errors.Is(err, ErrParse) {
		t.Errorf("WalkParts with a callback error = %v; want it as it is", err)
	}
}
