package mailsync

import (
	"io"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/compose"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/store"
)

// DraftMessage turns a draft into what compose.Build writes
// (docs/design/send.md, Building a message): From and Reply-To come from
// the identity, attachments are read from blobs, and quoted inline images,
// which the draft's HTML shows from mailpart: URLs into parts, become cid:
// parts. A mailpart: image missing from the cache is left out. date is the
// Date header.
func DraftMessage(d store.Draft, ident store.Identity, blobs *blob.Store, parts *render.PartsCache, date time.Time) compose.Message {
	c := d.Content
	m := compose.Message{
		From:       compose.Address{Name: ident.Name, Addr: ident.Email},
		To:         composeAddresses(c.To),
		Cc:         composeAddresses(c.Cc),
		Bcc:        composeAddresses(c.Bcc),
		Subject:    c.Subject,
		MessageID:  d.MessageID,
		InReplyTo:  d.InReplyTo,
		References: d.References,
		Date:       date,
	}
	if ident.ReplyTo != "" {
		m.ReplyTo = []compose.Address{{Addr: ident.ReplyTo}}
	}
	m.HTML, m.Inline = inlineParts(c.HTML, ident.Email, parts)
	for _, a := range d.Attachments {
		m.Attachments = append(m.Attachments, compose.Attachment{
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Open:        func() (io.ReadCloser, error) { return blobs.Open(a.BlobID) },
		})
	}
	return m
}

func composeAddresses(list []store.Address) []compose.Address {
	out := make([]compose.Address, 0, len(list))
	for _, a := range list {
		out = append(out, compose.Address{Name: a.Name, Addr: a.Addr})
	}
	return out
}

// inlineParts replaces every mailpart: URL in html with a cid: URL and
// returns the parts to send with it; one part per distinct URL. URLs whose
// file is not cached are replaced by an empty string.
func inlineParts(html, from string, parts *render.PartsCache) (string, []compose.Inline) {
	if parts == nil || !strings.Contains(html, render.PartURL) {
		return html, nil
	}
	var (
		b      strings.Builder
		inline []compose.Inline
		cids   = map[string]string{}
	)
	rest := html
	for {
		i := strings.Index(rest, render.PartURL)
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i+len(render.PartURL):]
		n := strings.IndexFunc(rest, func(r rune) bool { return !isPartPathRune(r) })
		if n < 0 {
			n = len(rest)
		}
		rel := rest[:n]
		rest = rest[n:]
		cid, seen := cids[rel]
		if !seen {
			if parts.Has(rel) {
				cid = compose.NewMessageID(from)
				inline = append(inline, compose.Inline{
					CID:         cid,
					ContentType: mime.TypeByExtension(path.Ext(rel)),
					Open:        func() (io.ReadCloser, error) { return parts.Open(rel) },
				})
			}
			cids[rel] = cid
		}
		if cid != "" {
			b.WriteString("cid:" + cid)
		}
	}
	return b.String(), inline
}

// isPartPathRune reports whether r can appear in a parts cache path as
// render writes them.
func isPartPathRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._/-", r)
}
