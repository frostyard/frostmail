package mimex

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/emersion/go-message/charset"
)

// DecodePart turns the leading bytes of a message text part into valid
// UTF-8 text for previews. data is still transfer-encoded (base64 or
// quoted-printable) in the given charset. truncated means data may end in
// the middle of an encoded unit, so an incomplete tail is dropped instead
// of reported as an error.
func DecodePart(data []byte, encoding, charset string, truncated bool) (string, error) {
	decoded, err := transferDecode(data, encoding, truncated)
	if err != nil {
		return "", err
	}
	text := toUTF8(decoded, charset)
	if truncated {
		text = dropIncompleteRune(text)
	}
	return strings.ToValidUTF8(text, "\uFFFD"), nil
}

// transferDecode undoes the transfer encoding named by encoding. Unknown
// encodings (7bit, 8bit, binary, empty) pass the bytes through.
func transferDecode(data []byte, encoding string, truncated bool) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		b := bytes.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, data)
		if truncated {
			b = b[:len(b)-len(b)%4]
		}
		decoded, err := base64.StdEncoding.DecodeString(string(b))
		if err != nil {
			return nil, fmt.Errorf("mimex: decode base64 part: %w", err)
		}
		return decoded, nil
	case "quoted-printable":
		return decodeQuotedPrintable(data, truncated), nil
	default:
		return data, nil
	}
}

// decodeQuotedPrintable decodes quoted-printable leniently: soft line
// breaks are removed, valid escapes become their byte, and with truncated
// set an incomplete escape at the very end is dropped. Any other '=' is
// kept as a literal '='.
func decodeQuotedPrintable(data []byte, truncated bool) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] != '=' {
			out = append(out, data[i])
			continue
		}
		switch {
		case i+1 < len(data) && data[i+1] == '\n':
			i++
		case i+2 < len(data) && data[i+1] == '\r' && data[i+2] == '\n':
			i += 2
		case i+2 < len(data) && isHexDigit(data[i+1]) && isHexDigit(data[i+2]):
			out = append(out, hexVal(data[i+1])<<4|hexVal(data[i+2]))
			i += 2
		case truncated && len(data)-i-1 < 2:
			return out
		default:
			out = append(out, '=')
		}
	}
	return out
}

// toUTF8 converts b from the named charset to a string. Charsets that are
// already UTF-8, and unknown or unreadable ones, yield the bytes as they
// are.
func toUTF8(b []byte, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "", "utf-8", "utf8", "us-ascii":
		return string(b)
	}
	r, err := charset.Reader(name, bytes.NewReader(b))
	if err != nil {
		return string(b)
	}
	converted, err := io.ReadAll(r)
	if err != nil {
		return string(b)
	}
	return string(converted)
}

// dropIncompleteRune removes a trailing incomplete UTF-8 sequence: it
// walks back over at most three continuation bytes to the start of the
// last rune and drops that rune if it is not complete.
func dropIncompleteRune(s string) string {
	b := []byte(s)
	if len(b) == 0 {
		return s
	}
	start := len(b)
	for i := len(b) - 1; i >= 0 && i > len(b)-4; i-- {
		if utf8.RuneStart(b[i]) {
			start = i
			break
		}
	}
	if start < len(b) && !utf8.FullRune(b[start:]) {
		return s[:start]
	}
	return s
}

func isHexDigit(b byte) bool {
	return ('0' <= b && b <= '9') || ('a' <= b && b <= 'f') || ('A' <= b && b <= 'F')
}

func hexVal(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10
	default:
		return b - 'A' + 10
	}
}
