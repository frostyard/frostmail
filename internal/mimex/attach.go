package mimex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
)

// Attachment is a part a reader shows as a file rather than as the body.
type Attachment struct {
	Filename    string // decoded from RFC 2231 and RFC 2047 forms
	ContentType string // lowercase type/subtype
	Size        int64  // decoded bytes (raw bytes for message/rfc822)
	ContentID   string // without angle brackets
	Inline      bool   // shown within the body (cid: images)
}

// Attachments lists a raw message's attachments in tree order. Every part
// is an attachment except multipart containers and text parts that carry
// no filename and are not marked attachment. A message truncated before
// its final boundary keeps the parts read before it, as BodyText does.
func Attachments(raw []byte) ([]Attachment, error) {
	root, err := message.Read(bytes.NewReader(raw))
	if err != nil && !isDecodingError(err) {
		return nil, fmt.Errorf("mimex: read message: %w", err)
	}
	if root == nil {
		return nil, errors.New("mimex: read message: no entity")
	}

	var atts []Attachment
	var visited bool
	err = root.Walk(func(_ []int, part *message.Entity, walkErr error) error {
		if part == nil || (walkErr != nil && !isDecodingError(walkErr)) {
			return nil
		}
		visited = true
		mediaType, params, _ := part.Header.ContentType()
		if strings.HasPrefix(mediaType, "multipart/") {
			return nil
		}
		disp, dispParams, _ := part.Header.ContentDisposition()
		filename := dispParams["filename"]
		if filename == "" {
			filename = params["name"]
		}
		if filename != "" {
			decoder := mime.WordDecoder{CharsetReader: charset.Reader}
			if decoded, err := decoder.DecodeHeader(filename); err == nil {
				filename = decoded
			}
		}
		if strings.HasPrefix(mediaType, "text/") && filename == "" && disp != "attachment" {
			return nil
		}
		// A truncated final part yields its bytes up to the cut; the
		// error is the truncation the walk rule says to keep.
		size, _ := io.Copy(io.Discard, part.Body)
		contentID := strings.Trim(part.Header.Get("Content-Id"), "<>")
		atts = append(atts, Attachment{
			Filename:    filename,
			ContentType: mediaType,
			Size:        size,
			ContentID:   contentID,
			Inline:      disp == "inline" || (disp == "" && contentID != ""),
		})
		return nil
	})
	if err != nil && !visited {
		return nil, err
	}
	return atts, nil
}
