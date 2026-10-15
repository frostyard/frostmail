package imapx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/frostyard/frostmail/internal/mimex"
)

// The display copy (ADR-0037): a message with the content of the parts a
// reader shows and an empty body for the rest, in the original's
// structure, so it parses as the original does.

// ErrNoDisplay means a message's structure cannot be made into a display
// copy (a multipart without a boundary, a part without a header): fetch
// the whole message instead.
var ErrNoDisplay = errors.New("imapx: no display copy")

// Limits on what a display copy fetches beyond text: a referenced or
// inline image, and an attached message.
const (
	displayImageLimit   = 4 << 20
	displayMessageLimit = 1 << 20
)

// FetchDisplay fetches the display copy of message uid in the selected
// mailbox: its BODYSTRUCTURE and header, then the MIME headers of its
// parts and the content of those a reader shows (shown).
func (s *Session) FetchDisplay(ctx context.Context, uid uint32) ([]byte, error) {
	header := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, Peek: true}
	var first []*imapclient.FetchMessageBuffer
	err := s.run(ctx, func() error {
		var err error
		first, err = s.c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{UID: true,
			BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
			BodySection:   []*imap.FetchItemBodySection{header}}).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fetch the structure of message %d: %w", uid, err)
	}
	if len(first) == 0 {
		return nil, ErrNoMessage
	}
	bs := first[0].BodyStructure
	if bs == nil {
		return nil, fmt.Errorf("%w: message %d has no body structure", ErrNoDisplay, uid)
	}
	plan := planDisplay(bs)
	var second []*imapclient.FetchMessageBuffer
	if len(plan.sections) > 0 {
		err = s.run(ctx, func() error {
			var err error
			second, err = s.c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{UID: true,
				BodySection: plan.sections}).Collect()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("fetch the parts of message %d: %w", uid, err)
		}
		if len(second) == 0 {
			return nil, ErrNoMessage
		}
	}
	found := func(sec *imap.FetchItemBodySection) []byte {
		if len(second) == 0 {
			return nil
		}
		return second[0].FindBodySection(sec)
	}
	var out bytes.Buffer
	if err := plan.write(&out, bs, nil, first[0].FindBodySection(header), found); err != nil {
		return nil, fmt.Errorf("%w: message %d: %w", ErrNoDisplay, uid, err)
	}
	return out.Bytes(), nil
}

// displayPlan is the sections a display copy fetches, by part path.
type displayPlan struct {
	sections []*imap.FetchItemBodySection
	mime     map[string]*imap.FetchItemBodySection
	content  map[string]*imap.FetchItemBodySection
}

func pathKey(path []int) string {
	parts := make([]string, len(path))
	for i, n := range path {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

func planDisplay(bs imap.BodyStructure) *displayPlan {
	p := &displayPlan{mime: map[string]*imap.FetchItemBodySection{}, content: map[string]*imap.FetchItemBodySection{}}
	add := func(m map[string]*imap.FetchItemBodySection, key string, sec *imap.FetchItemBodySection) {
		m[key] = sec
		p.sections = append(p.sections, sec)
	}
	// A message that is one part has its own header and its body as TEXT;
	// Walk would call it part 1.
	if sp, ok := bs.(*imap.BodyStructureSinglePart); ok {
		if shown(sp) {
			add(p.content, "", &imap.FetchItemBodySection{Specifier: imap.PartSpecifierText, Peek: true})
		}
		return p
	}
	bs.Walk(func(path []int, part imap.BodyStructure) bool {
		key := pathKey(path)
		if len(path) > 0 {
			add(p.mime, key, &imap.FetchItemBodySection{Specifier: imap.PartSpecifierMIME, Part: path, Peek: true})
		}
		sp, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return true
		}
		if shown(sp) {
			add(p.content, key, &imap.FetchItemBodySection{Part: path, Peek: true})
		}
		return false // a message/rfc822 part is shown or omitted whole
	})
	return p
}

// shown reports whether a display copy fetches a part's content: text
// that is not an attachment, by the rule sync uses for has_attachments;
// an invitation; a referenced or inline image; an attached message.
func shown(sp *imap.BodyStructureSinglePart) bool {
	typ := strings.ToLower(sp.Type + "/" + sp.Subtype)
	name := strings.ToLower(sp.Filename())
	disposition := ""
	if sp.Extended != nil && sp.Extended.Disposition != nil {
		disposition = strings.ToLower(sp.Extended.Disposition.Value)
	}
	attached := disposition == "attachment" || (name != "" && disposition != "inline")
	switch {
	case typ == "text/calendar" || typ == "application/ics" || strings.HasSuffix(name, ".ics"):
		return true
	case strings.HasPrefix(typ, "message/"):
		return sp.Size <= displayMessageLimit
	case sp.ID != "" || (!attached && strings.HasPrefix(typ, "image/")):
		return sp.Size <= displayImageLimit
	case strings.HasPrefix(typ, "text/"):
		return !attached
	}
	return false
}

// write writes the part at path: its header (the message's own for the
// root, else its MIME header), then its children between its boundary,
// or its content, or nothing and the omitted mark.
func (p *displayPlan) write(out *bytes.Buffer, bs imap.BodyStructure, path []int, header []byte, found func(*imap.FetchItemBodySection) []byte) error {
	key := pathKey(path)
	if len(path) > 0 {
		header = found(p.mime[key])
	}
	if len(header) == 0 {
		return fmt.Errorf("no header for part %q", key)
	}
	switch part := bs.(type) {
	case *imap.BodyStructureMultiPart:
		out.Write(header)
		boundary := ""
		if part.Extended != nil {
			boundary = part.Extended.Params["boundary"]
		}
		if boundary == "" {
			return fmt.Errorf("multipart %q has no boundary", key)
		}
		for i, child := range part.Children {
			if i == 0 {
				out.WriteString("--" + boundary + "\r\n")
			} else {
				out.WriteString("\r\n--" + boundary + "\r\n")
			}
			if err := p.write(out, child, append(append([]int(nil), path...), i+1), nil, found); err != nil {
				return err
			}
		}
		out.WriteString("\r\n--" + boundary + "--\r\n")
	case *imap.BodyStructureSinglePart:
		sec, ok := p.content[key]
		if !ok {
			out.Write(omitted(header, part.Size))
			return nil
		}
		out.Write(header)
		out.Write(found(sec))
	default:
		return fmt.Errorf("part %q has an unknown structure", key)
	}
	return nil
}

// omitted is a part's header with the omitted mark before its blank line.
func omitted(header []byte, size uint32) []byte {
	mark := mimex.OmittedHeader + ": " + strconv.FormatUint(uint64(size), 10) + "\r\n"
	end := bytes.LastIndex(header, []byte("\r\n\r\n"))
	if end < 0 {
		return append(append([]byte(nil), header...), []byte("\r\n"+mark+"\r\n")...)
	}
	out := append([]byte(nil), header[:end+2]...)
	out = append(out, mark...)
	return append(out, header[end+2:]...)
}
