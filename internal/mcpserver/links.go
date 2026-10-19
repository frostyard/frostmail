package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// A message's link: its Message-ID header and its permanent lodge URL
// (docs/specs/lodge-app.md, Routes), which the tools answer so an agent
// links mail by a route that lasts and never encodes one itself.

// messageURL is the permanent lodge URL of a message by its Message-ID,
// without angle brackets: /mail/message/ and the Message-ID percent-encoded
// as one segment, as the web app's encodeURIComponent does. It is "" for no
// Message-ID, and for one the route would read as maild's own ID (digits
// alone), which does not last.
func messageURL(messageID string) string {
	if strings.TrimSpace(messageID) == "" || maildID(messageID) {
		return ""
	}
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.WriteString("/mail/message/")
	for i := range len(messageID) {
		c := messageID[i]
		if componentSafe(c) {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// componentSafe reports whether encodeURIComponent leaves c as it is.
func componentSafe(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-_.!~*'()", c) >= 0
}

// maildID reports whether a route segment reads as maild's ID: 1 to 16
// digits without a leading zero, as the web app's routes take one.
func maildID(s string) bool {
	if len(s) == 0 || len(s) > 16 || s[0] == '0' {
		return false
	}
	return strings.Trim(s, "0123456789") == ""
}

// messageLink is a message's Message-ID header and URL; both are "" for a
// message without a Message-ID.
func messageLink(messageID string) (header, url string) {
	if strings.TrimSpace(messageID) == "" {
		return "", ""
	}
	return messageID, messageURL(messageID)
}

// messageIDHeaders reads the Message-ID of each message; a message gone
// since its ID was read is left out.
func (t *tools) messageIDHeaders(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		m, err := t.o.Client.Message().Get(ctx, &api.MessageGetParams{ID: id})
		if errors.Is(err, api.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = m.MessageID
	}
	return out, nil
}
