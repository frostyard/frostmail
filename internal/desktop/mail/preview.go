package mail

import (
	"path/filepath"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// MaxPreviewBytes is the largest attachment the reader makes a thumbnail
// of, by the size maild names for the part.
const MaxPreviewBytes = 16 << 20

// MaxPreviews is how many thumbnails one message gets; further pictures
// stay chips.
const MaxPreviews = 12

// previewTypes are the pictures the reader makes thumbnails of: the
// formats it decodes. SVG is not one: it is a document, not pixels.
var previewTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/bmp": true}

var previewExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true}

// Previewable reports whether an attachment may get a thumbnail: a
// picture by its type, or by its name when the sender gave no type, of a
// size worth fetching unasked.
func Previewable(p api.Part) bool {
	if p.Size <= 0 || p.Size > MaxPreviewBytes {
		return false
	}
	ctype := strings.ToLower(p.ContentType)
	if previewTypes[ctype] {
		return true
	}
	return (ctype == "application/octet-stream" || ctype == "") && previewExtensions[strings.ToLower(filepath.Ext(p.Filename))]
}

// Previews are the attachments of a message that get thumbnails, in
// order, at most MaxPreviews.
func Previews(attachments []api.Part) []api.Part {
	var out []api.Part
	for _, p := range attachments {
		if len(out) < MaxPreviews && Previewable(p) {
			out = append(out, p)
		}
	}
	return out
}

// PreviewSize is the size, in DIPs, a thumbnail of w × h pixels shows at:
// inside maxW × maxH, never larger than the picture.
func PreviewSize(w, h int, maxW, maxH float32) (float32, float32) {
	fw, fh := float32(w), float32(h)
	if fw <= 0 || fh <= 0 {
		return 0, 0
	}
	switch {
	case fw <= maxW && fh <= maxH:
		return fw, fh
	case fw*maxH > fh*maxW: // the width decides
		return maxW, fh * maxW / fw
	}
	return fw * maxH / fh, maxH
}
