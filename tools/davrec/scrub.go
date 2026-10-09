package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostmail/internal/httprec"
)

type scrubber struct {
	key                       []byte
	secrets, values, excluded map[string]bool
}

func newScrubber(key []byte) *scrubber {
	return &scrubber{key: key, secrets: map[string]bool{}, values: map[string]bool{}, excluded: map[string]bool{}}
}

type unit struct {
	r       rune
	raw     string
	percent int
	entity  bool
	escape  bool
}

// dates are kept as they are, being what a sync orders and expands by:
// ISO 8601 and iCalendar dates (and times) with a real month and day, so a
// number that only looks like one, a phone number say, is still replaced.
var dates = regexp.MustCompile(`\b(?:1\d|20)\d{2}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])(?:T[0-9:.]+Z)?\b|` +
	`\b(?:1\d|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])(?:T\d{6}Z?)?\b`)

// zones are IANA time zone names, kept as dates are: they place events.
var zones = regexp.MustCompile(`\b(?:Africa|America|Antarctica|Arctic|Asia|Atlantic|Australia|Europe|Indian|Pacific|Etc)` +
	`/[A-Za-z0-9_+-]+(?:/[A-Za-z0-9_+-]+)?\b`)

// kept are the spans of text that scrubbing leaves as they are.
func kept(text string) [][]int {
	return append(dates.FindAllStringIndex(text, -1), zones.FindAllStringIndex(text, -1)...)
}

func percentByte(b byte) string { return fmt.Sprintf("%%%02X", b) }

// units decodes escapes for matching while retaining their original bytes.
func units(text string) []unit {
	var out []unit
	for len(text) > 0 {
		u, n := nextUnit(text)
		out = append(out, u)
		text = text[n:]
	}
	return out
}

func nextUnit(text string) (unit, int) {
	r, n := utf8.DecodeRuneInString(text)
	u := unit{r: r, raw: text[:n]}
	if text[0] == '%' {
		at := 1
		for strings.HasPrefix(text[at:], "25") && len(text) >= at+4 {
			u.percent++
			at += 2
		}
		if len(text) >= at+2 {
			if v, err := strconv.ParseUint(text[at:at+2], 16, 8); err == nil {
				u.r, n, u.percent = rune(v), at+2, u.percent+1
				u.raw = text[:n]
				return u, n
			}
		}
		u.percent = 0
	}
	if text[0] == '&' {
		if end := strings.IndexByte(text, ';'); end > 0 && end < 16 {
			raw := text[:end+1]
			value := html.UnescapeString(raw)
			if value != raw && utf8.RuneCountInString(value) == 1 {
				u.r, _ = utf8.DecodeRuneInString(value)
				u.raw, u.entity = raw, true
				return u, end + 1
			}
		}
	}
	if text[0] == '\\' && len(text) > 1 {
		n = 2
		if text[1] == 'u' && len(text) >= 6 {
			n = 6
		}
		raw := text[:n]
		if value, err := strconv.Unquote(`"` + raw + `"`); err == nil {
			u.r, _ = utf8.DecodeRuneInString(value)
		} else {
			u.r = rune(text[1])
			n = 2
		}
		u.raw, u.escape = text[:n], true
		return u, n
	}
	return u, n
}

func decoded(text string) string {
	var b strings.Builder
	for _, u := range units(text) {
		b.WriteRune(u.r)
	}
	return b.String()
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func words(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return !wordRune(r) })
}

func (s *scrubber) fake(word string) string {
	lower := strings.ToLower(word)
	for attempt := 0; ; attempt++ {
		mac := hmac.New(sha256.New, s.key)
		fmt.Fprintf(mac, "%d:%s", attempt, lower)
		sum := mac.Sum(nil)
		out := []rune(word)
		for i, r := range out {
			if i > 0 && i%len(sum) == 0 {
				_, _ = mac.Write(sum)
				sum = mac.Sum(nil)
			}
			v := rune(sum[i%len(sum)])
			switch {
			case unicode.IsDigit(r):
				out[i] = '0' + v%10
			case unicode.IsUpper(r):
				out[i] = 'A' + v%26
			default:
				out[i] = 'a' + v%26
			}
		}
		f := string(out)
		if !vocabulary[strings.ToLower(f)] && !s.excluded[strings.ToLower(f)] && !strings.EqualFold(f, word) {
			return f
		}
	}
}

func (u unit) render(r rune) string {
	if r == u.r {
		return u.raw
	}
	if u.percent > 0 {
		out := ""
		for _, b := range []byte(string(r)) {
			out += "%" + strings.Repeat("25", u.percent-1) + fmt.Sprintf("%02X", b)
		}
		return out
	}
	if u.entity {
		return fmt.Sprintf("&#x%X;", r)
	}
	if u.escape {
		if strings.HasPrefix(u.raw, `\u`) {
			return fmt.Sprintf(`\u%04X`, r)
		}
		// Content-line escapes may quote any character, unlike JSON escapes.
		if strings.ContainsRune(`"\/bfnrt`, u.r) {
			return string(r)
		}
		return `\` + string(r)
	}
	return string(r)
}

// text replaces complete decoded words in a single pass, so a fake never
// becomes input to another replacement and substrings cannot collide.
func (s *scrubber) text(text string) string {
	us := units(text)
	plain := decoded(text)
	protected := kept(plain)
	var b strings.Builder
	offset := 0
	for i := 0; i < len(us); {
		if !wordRune(us[i].r) {
			b.WriteString(us[i].raw)
			offset += utf8.RuneLen(us[i].r)
			i++
			continue
		}
		j := i
		var w strings.Builder
		for j < len(us) && wordRune(us[j].r) {
			w.WriteRune(us[j].r)
			j++
		}
		word := w.String()
		lower := strings.ToLower(word)
		keep := false
		for _, span := range protected {
			if offset >= span[0] && offset < span[1] {
				keep = true
			}
		}
		if s.secrets[lower] && !vocabulary[lower] && !s.excluded[lower] && !keep {
			fake := []rune(s.fake(word))
			for k := i; k < j; k++ {
				b.WriteString(us[k].render(fake[k-i]))
			}
		} else {
			for k := i; k < j; k++ {
				b.WriteString(us[k].raw)
			}
		}
		offset += len(word)
		i = j
	}
	return b.String()
}

func (s *scrubber) header(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, vs := range h {
		key := s.text(k)
		for _, v := range vs {
			out[key] = append(out[key], s.text(v))
		}
	}
	return out
}

func (s *scrubber) replace(xs []httprec.Exchange) error {
	for i := range xs {
		x := &xs[i]
		x.URL, x.Err = s.text(x.URL), s.text(x.Err)
		x.ReqHeader, x.RespHeader = s.header(x.ReqHeader), s.header(x.RespHeader)
		if !x.ReqBase64 {
			x.ReqBody = s.text(x.ReqBody)
		}
		if !x.RespBase64 {
			x.RespBody = s.text(x.RespBody)
		}
	}
	found := map[string]bool{}
	for _, x := range xs {
		for _, text := range exchangeText(x) {
			s.leaks(text, found)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("scrubbing refused: %d personal values or words remain", len(found))
	}
	return nil
}

// maskKept blanks the dates and zones text keeps, keeping every other
// offset.
func maskKept(text string) string {
	b := []byte(text)
	for _, span := range kept(text) {
		for i := span[0]; i < span[1]; i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

// leaks adds the personal values and words left in text to found. The
// dates and zones text keeps are not leaks, though a value or word holds
// them (a birthday, a UID that starts with its event's date, a calendar's
// zone).
func (s *scrubber) leaks(text string, found map[string]bool) {
	text = strings.ToLower(maskKept(decoded(text)))
	for value := range s.values {
		if strings.Contains(text, value) {
			found[value] = true
		}
	}
	for _, word := range words(text) {
		if len([]rune(word)) >= 6 && s.secrets[word] && !vocabulary[word] && !s.excluded[word] {
			found[word] = true
		}
	}
}
