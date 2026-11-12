package mimex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

// Body returns a raw RFC 5322 message's readable body in one parse of its
// MIME tree: its text, the HTML part that text may come from, and whether the
// message carries HTML. BodyText and BodyHTML are its halves.
func Body(raw []byte) (text, html string, hasHTML bool, err error) {
	root, err := message.Read(bytes.NewReader(raw))
	if err != nil && !isDecodingError(err) {
		return "", "", false, parseFailed(fmt.Errorf("mimex: read message: %w", err))
	}
	if root == nil {
		return "", "", false, parseFailed(errors.New("mimex: read message: no entity"))
	}

	var parts bodyParts
	if err := root.Walk(parts.visit); err != nil && !parts.visited {
		return "", "", false, parseFailed(err)
	}

	text = parts.plain
	if strings.TrimSpace(text) == "" {
		text = HTMLToText(parts.html)
	}
	return tidyLines(text), parts.html, parts.hasHTML, nil
}

// BodyText returns the readable text of a raw RFC 5322 message and whether
// the message carries an HTML part. The text is the first text/plain part
// when it is not blank, and otherwise text made from the first text/html
// part. Attachments and forwarded messages are never the body.
func BodyText(raw []byte) (string, bool, error) {
	text, _, hasHTML, err := Body(raw)
	if err != nil {
		return "", false, err
	}
	return text, hasHTML, nil
}

// BodyHTML returns the HTML part BodyText would derive text from: the first
// text/html part that is not an attachment or a forwarded message, decoded
// to UTF-8, or "" when the message has none.
func BodyHTML(raw []byte) (string, error) {
	_, html, _, err := Body(raw)
	return html, err
}

// bodyParts accumulates the first text/plain and the first text/html part
// found while walking a message's MIME tree.
type bodyParts struct {
	plain    string
	html     string
	hasPlain bool
	hasHTML  bool
	visited  bool
}

// visit records one walked part. Parts go-message could not decode are
// skipped, except that an unknown charset still yields the bytes as read.
func (b *bodyParts) visit(_ []int, part *message.Entity, walkErr error) error {
	if part == nil || (walkErr != nil && !isDecodingError(walkErr)) {
		return nil
	}
	b.visited = true
	mediaType, params, _ := part.Header.ContentType()
	if notBodyText(mediaType, params, &part.Header) {
		return nil
	}
	switch mediaType {
	case "text/plain":
		if b.hasPlain {
			return nil
		}
		text, err := readBody(part)
		if err != nil {
			return err
		}
		b.plain, b.hasPlain = text, true
	case "text/html":
		if b.hasHTML {
			return nil
		}
		text, err := readBody(part)
		if err != nil {
			return err
		}
		b.html, b.hasHTML = text, true
	}
	return nil
}

// notBodyText reports whether the part is a multipart container, a
// forwarded message, or an attachment rather than readable body text. A
// part is an attachment when its disposition says so, or when it is named
// but not marked inline.
func notBodyText(mediaType string, params map[string]string, header *message.Header) bool {
	if strings.HasPrefix(mediaType, "multipart/") || mediaType == "message/rfc822" {
		return true
	}
	disp, dispParams, _ := header.ContentDisposition()
	if disp == "attachment" {
		return true
	}
	return disp != "inline" && (dispParams["filename"] != "" || params["name"] != "")
}

// readBody reads a part's decoded body. An unknown charset or encoding
// leaves the bytes as read.
func readBody(part *message.Entity) (string, error) {
	b, err := io.ReadAll(part.Body)
	if err != nil && !isDecodingError(err) {
		return "", fmt.Errorf("mimex: read part: %w", err)
	}
	return string(b), nil
}

// isDecodingError reports whether err only says go-message does not know
// the charset or transfer encoding, which still leaves the bytes readable.
func isDecodingError(err error) bool {
	return message.IsUnknownCharset(err) || message.IsUnknownEncoding(err)
}

// tidyLines normalizes line endings, trims trailing whitespace from every
// line except the signature separator "-- " (RFC 3676, 4.3), and drops the
// leading and trailing blank lines so the result begins and ends with text.
func tidyLines(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if line == "-- " {
			continue
		}
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}
