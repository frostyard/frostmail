package mimex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
)

// PartInfo describes one leaf part of a message.
type PartInfo struct {
	Path        string // IMAP part specifier, as in BODYSTRUCTURE: "1", "2.1"
	ContentType string // lowercase type/subtype
	Disposition string // lowercase; "" when absent
	Filename    string // decoded from RFC 2231 and RFC 2047 forms
	ContentID   string // without angle brackets
}

// WalkParts calls fn for every leaf part of a raw message in tree order, with
// the part's body decoded from its transfer encoding; text parts in a known
// charset are also converted to UTF-8. A message that is not multipart is the single part
// "1"; parts of a multipart are numbered from 1 per level, joined with dots.
// Forwarded messages (message/rfc822) are one leaf. fn's error stops the walk
// and is returned.
func WalkParts(raw []byte, fn func(p PartInfo, body io.Reader) error) error {
	root, err := message.Read(bytes.NewReader(raw))
	if err != nil && !isDecodingError(err) {
		return fmt.Errorf("mimex: read message: %w", err)
	}
	if root == nil {
		return errors.New("mimex: read message: no entity")
	}
	var stop error
	walkErr := root.Walk(func(path []int, part *message.Entity, err error) error {
		if part == nil || (err != nil && !isDecodingError(err)) {
			return nil
		}
		mediaType, params, _ := part.Header.ContentType()
		if strings.HasPrefix(mediaType, "multipart/") {
			return nil
		}
		disp, dispParams, _ := part.Header.ContentDisposition()
		name := dispParams["filename"]
		if name == "" {
			name = params["name"]
		}
		if name != "" {
			dec := mime.WordDecoder{CharsetReader: charset.Reader}
			if d, err := dec.DecodeHeader(name); err == nil {
				name = d
			}
		}
		info := PartInfo{
			Path:        partPath(path),
			ContentType: strings.ToLower(mediaType),
			Disposition: strings.ToLower(disp),
			Filename:    name,
			ContentID:   strings.Trim(strings.TrimSpace(part.Header.Get("Content-Id")), "<>"),
		}
		if err := fn(info, part.Body); err != nil {
			stop = err
			return err
		}
		return nil
	})
	if stop != nil {
		return stop
	}
	if walkErr != nil && !isDecodingError(walkErr) {
		return walkErr
	}
	return nil
}

// partPath turns go-message's child indexes into an IMAP part specifier.
func partPath(path []int) string {
	if len(path) == 0 {
		return "1"
	}
	parts := make([]string, len(path))
	for i, n := range path {
		parts[i] = strconv.Itoa(n + 1)
	}
	return strings.Join(parts, ".")
}
