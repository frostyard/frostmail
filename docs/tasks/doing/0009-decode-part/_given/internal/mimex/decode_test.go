package mimex

// CONTRACT TEST for task card T-0009 (docs/tasks). Do not edit.

import (
	"testing"
	"unicode/utf8"
)

func TestDecodePart(t *testing.T) {
	cases := []struct {
		name              string
		data              string
		encoding, charset string
		truncated         bool
		want              string
	}{
		{"base64", "SGVsbG8gV29ybGQ=", "base64", "utf-8", false, "Hello World"},
		{"base64 lines and case", "SGVs\r\nbG8g\r\nV29y\r\nbGQ=", "BASE64", "", false, "Hello World"},
		{"base64 truncated", "SGVsbG8gV29ybG", "base64", "utf-8", true, "Hello Wor"},
		{"base64 truncated with a line break", "SGVsbG8g\r\nV29ybG", "base64", "utf-8", true, "Hello Wor"},
		{"base64 latin1", "Y2Fm6Q==", "base64", "iso-8859-1", false, "café"},
		{"qp", "caf=C3=A9 au lait=\r\n continued", "quoted-printable", "utf-8", false, "café au lait continued"},
		{"qp lowercase hex", "caf=c3=a9", "Quoted-Printable", "utf-8", false, "café"},
		{"qp invalid escape kept", "100=ZZ sure", "quoted-printable", "utf-8", false, "100=ZZ sure"},
		{"qp truncated", "price =E2=82=AC5 =E2=8", "quoted-printable", "utf-8", true, "price €5 "},
		{"qp soft break at end", "abc=", "quoted-printable", "utf-8", true, "abc"},
		{"8bit latin1", "caf\xe9", "8bit", "iso-8859-1", false, "café"},
		{"windows-1252 quotes", "\x93quoted\x94", "7bit", "windows-1252", false, "“quoted”"},
		{"shift_jis", "\x93\xfa\x96\x7b", "8bit", "shift_jis", false, "日本"},
		{"koi8-r", "\xf0\xd2\xc9\xd7\xc5\xd4", "8bit", "KOI8-R", false, "Привет"},
		{"gb2312", "\xc4\xe3\xba\xc3", "8bit", "gb2312", false, "你好"},
		{"utf8 alias", "naïve", "", "UTF8", false, "naïve"},
		{"unknown charset", "hello", "7bit", "x-made-up", false, "hello"},
		{"unknown encoding", "plain", "x-uuencode", "", false, "plain"},
		{"binary", "bytes", "binary", "us-ascii", false, "bytes"},
		{"invalid utf8", "ok\xff", "8bit", "utf-8", false, "ok�"},
		{"truncated utf8", "caf\xc3", "8bit", "utf-8", true, "caf"},
		{"truncated utf8 not flagged", "caf\xc3", "8bit", "utf-8", false, "caf�"},
		{"empty", "", "base64", "utf-8", false, ""},
	}
	for _, tc := range cases {
		got, err := DecodePart([]byte(tc.data), tc.encoding, tc.charset, tc.truncated)
		if err != nil {
			t.Errorf("%s: DecodePart error %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: DecodePart = %q, want %q", tc.name, got, tc.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: result is not valid UTF-8", tc.name)
		}
	}
}

func TestDecodePartErrors(t *testing.T) {
	for _, data := range []string{"SGVsbG8gV29ybG", "SGV$bG8=", "S"} {
		if _, err := DecodePart([]byte(data), "base64", "utf-8", false); err == nil {
			t.Errorf("DecodePart(%q, base64, complete) succeeded; want an error", data)
		}
	}
}
