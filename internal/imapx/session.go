package imapx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/store"
)

// PreviewBytes is how much of a message's text part a header fetch reads
// for its preview.
const PreviewBytes = 2048

// PreviewRunes is the length of a stored preview.
const PreviewRunes = 200

// Capabilities are the server extensions sync uses, probed once after login.
type Capabilities struct {
	CondStore    bool
	ESearch      bool
	Binary       bool
	Move         bool
	UIDPlus      bool
	Idle         bool
	SpecialUse   bool
	ListExtended bool
	// Gmail is X-GM-EXT-1: X-GM-MSGID, X-GM-THRID, X-GM-LABELS (ADR-0012).
	Gmail bool
}

// Session is one logged-in IMAP connection. It is not safe for concurrent
// use: the account actor drives it from one goroutine. Every method takes a
// context; when the context ends mid-command the connection is closed and
// the session must be discarded.
type Session struct {
	c        *imapclient.Client
	Caps     Capabilities
	selected string
}

// Open dials, logs in and probes capabilities.
// opts.OnUpdate, if set, is called for every unsolicited EXISTS, EXPUNGE or
// FETCH, from the connection's reader goroutine; it must not block.
func Open(ctx context.Context, opts DialOptions) (*Session, error) {
	c, err := Dial(ctx, opts)
	if err != nil {
		return nil, err
	}
	s := &Session{c: c}
	err = s.run(ctx, func() error {
		caps, err := c.Capability().Wait()
		if err != nil {
			return fmt.Errorf("capability: %w", err)
		}
		s.Caps = Capabilities{
			CondStore:    caps.Has(imap.CapCondStore),
			ESearch:      caps.Has(imap.CapESearch),
			Binary:       caps.Has(imap.CapBinary),
			Move:         caps.Has(imap.CapMove),
			UIDPlus:      caps.Has(imap.CapUIDPlus),
			Idle:         caps.Has(imap.CapIdle),
			SpecialUse:   caps.Has(imap.CapSpecialUse),
			ListExtended: caps.Has(imap.CapListExtended),
			Gmail:        caps.Has("X-GM-EXT-1"),
		}
		if caps.Has(imap.CapID) {
			// Some servers require ID; nothing depends on the answer.
			_, _ = c.ID(&imap.IDData{Name: "frostmail", Vendor: "frostyard"}).Wait()
		}
		// No ENABLE CONDSTORE: SELECT (CONDSTORE) enables it implicitly
		// (RFC 7162 3.1), and go-imap only ENABLEs extensions it handles.
		return nil
	})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return s, nil
}

// Close logs out (best effort) and closes the connection.
func (s *Session) Close() error {
	done := make(chan struct{})
	go func() {
		_ = s.c.Logout().Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	return s.c.Close()
}

// run executes f, closing the connection if ctx ends first so a blocked
// command returns.
func (s *Session) run(ctx context.Context, f func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = s.c.Close() })
	err := f()
	if !stop() {
		return ctx.Err()
	}
	return err
}

// List returns every mailbox with its role resolved (RoleFor).
func (s *Session) List(ctx context.Context) ([]store.ServerMailbox, error) {
	var list []*imap.ListData
	err := s.run(ctx, func() error {
		var opts *imap.ListOptions
		if s.Caps.ListExtended {
			opts = &imap.ListOptions{ReturnSubscribed: true, ReturnSpecialUse: s.Caps.SpecialUse}
		}
		var err error
		list, err = s.c.List("", "*", opts).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	out := make([]store.ServerMailbox, 0, len(list))
	taken := map[api.MailboxRole]bool{}
	for _, l := range list {
		mb := store.ServerMailbox{Path: l.Mailbox, Selectable: true, Subscribed: !s.Caps.ListExtended}
		if l.Delim != 0 {
			mb.Delimiter = string(l.Delim)
		}
		for _, a := range l.Attrs {
			mb.Attrs = append(mb.Attrs, string(a))
			switch strings.ToLower(string(a)) {
			case `\noselect`, `\nonexistent`:
				mb.Selectable = false
			case `\subscribed`:
				mb.Subscribed = true
			}
		}
		if l.ChildInfo != nil && l.ChildInfo.Subscribed || strings.EqualFold(mb.Path, "INBOX") {
			mb.Subscribed = true // INBOX always shows, subscribed or not
		}
		role := RoleFor(mb.Path, mb.Delimiter, mb.Attrs)
		if role != api.MailboxRoleNone && taken[role] {
			role = api.MailboxRoleNone
		}
		taken[role] = true
		mb.Role = role
		out = append(out, mb)
	}
	return out, nil
}

// RoleFor resolves a mailbox's role from its SPECIAL-USE attributes, then
// from common names (top-level only).
func RoleFor(path, delim string, attrs []string) api.MailboxRole {
	if strings.EqualFold(path, "INBOX") {
		return api.MailboxRoleInbox
	}
	for _, a := range attrs {
		switch strings.ToLower(a) {
		case `\sent`:
			return api.MailboxRoleSent
		case `\drafts`:
			return api.MailboxRoleDrafts
		case `\trash`:
			return api.MailboxRoleTrash
		case `\junk`:
			return api.MailboxRoleJunk
		case `\archive`:
			return api.MailboxRoleArchive
		case `\all`:
			return api.MailboxRoleAll
		case `\flagged`:
			return api.MailboxRoleFlagged
		}
	}
	if delim != "" && strings.Contains(path, delim) {
		return api.MailboxRoleNone
	}
	switch strings.ToLower(path) {
	case "sent", "sent messages", "sent items", "sent mail":
		return api.MailboxRoleSent
	case "drafts", "draft":
		return api.MailboxRoleDrafts
	case "trash", "deleted messages", "deleted items", "bin":
		return api.MailboxRoleTrash
	case "junk", "spam", "junk e-mail", "junk email", "bulk mail":
		return api.MailboxRoleJunk
	case "archive", "archives":
		return api.MailboxRoleArchive
	}
	return api.MailboxRoleNone
}

// Selected is what SELECT reports.
type Selected struct {
	UIDValidity   uint32
	UIDNext       uint32
	HighestModSeq uint64
	Messages      uint32
}

// Select opens a mailbox read-write.
func (s *Session) Select(ctx context.Context, path string) (Selected, error) {
	var d *imap.SelectData
	err := s.run(ctx, func() error {
		var err error
		d, err = s.c.Select(path, &imap.SelectOptions{CondStore: s.Caps.CondStore}).Wait()
		return err
	})
	if err != nil {
		s.selected = ""
		return Selected{}, fmt.Errorf("select %s: %w", path, err)
	}
	s.selected = path
	return Selected{UIDValidity: d.UIDValidity, UIDNext: uint32(d.UIDNext), HighestModSeq: d.HighestModSeq, Messages: d.NumMessages}, nil
}

// Status reports a mailbox's state without selecting it.
func (s *Session) Status(ctx context.Context, path string) (Selected, error) {
	var d *imap.StatusData
	err := s.run(ctx, func() error {
		var err error
		d, err = s.c.Status(path, &imap.StatusOptions{
			NumMessages: true, UIDNext: true, UIDValidity: true, HighestModSeq: s.Caps.CondStore,
		}).Wait()
		return err
	})
	if err != nil {
		return Selected{}, fmt.Errorf("status %s: %w", path, err)
	}
	out := Selected{UIDValidity: d.UIDValidity, UIDNext: uint32(d.UIDNext), HighestModSeq: d.HighestModSeq}
	if d.NumMessages != nil {
		out.Messages = *d.NumMessages
	}
	return out, nil
}

// UIDs returns every UID in the selected mailbox, ascending.
func (s *Session) UIDs(ctx context.Context) ([]uint32, error) {
	var d *imap.SearchData
	err := s.run(ctx, func() error {
		var opts *imap.SearchOptions
		if s.Caps.ESearch {
			opts = &imap.SearchOptions{ReturnAll: true}
		}
		var err error
		d, err = s.c.UIDSearch(&imap.SearchCriteria{}, opts).Wait()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("uid search: %w", err)
	}
	all := d.AllUIDs()
	out := make([]uint32, len(all))
	for i, u := range all {
		out[i] = uint32(u)
	}
	slices.Sort(out)
	return out, nil
}

// SearchMessageID returns the UIDs in the selected mailbox whose Message-ID
// header contains msgid (given without angle brackets), ascending.
func (s *Session) SearchMessageID(ctx context.Context, msgid string) ([]uint32, error) {
	var d *imap.SearchData
	err := s.run(ctx, func() error {
		criteria := &imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Message-ID", Value: msgid}}}
		var err error
		d, err = s.c.UIDSearch(criteria, nil).Wait()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("uid search message-id: %w", err)
	}
	all := d.AllUIDs()
	out := make([]uint32, len(all))
	for i, u := range all {
		out[i] = uint32(u)
	}
	slices.Sort(out)
	return out, nil
}

// uidSet compresses UIDs into ranges, keeping commands short.
func uidSet(uids []uint32) imap.UIDSet {
	sorted := slices.Clone(uids)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	var set imap.UIDSet
	for i := 0; i < len(sorted); {
		j := i
		for j+1 < len(sorted) && sorted[j+1] == sorted[j]+1 {
			j++
		}
		set = append(set, imap.UIDRange{Start: imap.UID(sorted[i]), Stop: imap.UID(sorted[j])})
		i = j + 1
	}
	return set
}

var extraHeaders = []string{"References", "List-Id", "List-Unsubscribe", "Authentication-Results"}

// FetchHeaders fetches headers, structure, flags and a preview for uids in
// the selected mailbox. UIDs the server no longer has are missing from the
// result.
func (s *Session) FetchHeaders(ctx context.Context, uids []uint32) ([]store.MessageHeader, error) {
	return s.fetchHeaders(ctx, uids, false)
}

// FetchGmailHeaders is FetchHeaders plus X-GM-MSGID, X-GM-THRID and
// X-GM-LABELS (Caps.Gmail).
func (s *Session) FetchGmailHeaders(ctx context.Context, uids []uint32) ([]store.MessageHeader, error) {
	return s.fetchHeaders(ctx, uids, true)
}

func (s *Session) fetchHeaders(ctx context.Context, uids []uint32, gmail bool) ([]store.MessageHeader, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	headerSection := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: extraHeaders, Peek: true}
	var msgs []*imapclient.FetchMessageBuffer
	err := s.run(ctx, func() error {
		var err error
		msgs, err = s.c.Fetch(uidSet(uids), &imap.FetchOptions{
			UID: true, Flags: true, InternalDate: true, RFC822Size: true, Envelope: true,
			BodyStructure: &imap.FetchItemBodyStructure{Extended: true}, ModSeq: s.Caps.CondStore,
			BodySection: []*imap.FetchItemBodySection{headerSection},
			GmailMsgID:  gmail, GmailThreadID: gmail, GmailLabels: gmail,
		}).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fetch headers: %w", err)
	}
	out := make([]store.MessageHeader, 0, len(msgs))
	previewPart := map[uint32]Part{}
	for _, m := range msgs {
		h, part := convertHeader(m, m.FindBodySection(headerSection))
		if gmail {
			h.GmMsgID, h.GmThrID, h.Labels = m.GmailMsgID, m.GmailThreadID, nonNilLabels(m.GmailLabels)
		}
		out = append(out, h)
		if part.Path != "" {
			previewPart[h.UID] = part
		}
	}
	previews, err := s.fetchPreviews(ctx, previewPart)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Preview = previews[out[i].UID]
	}
	return out, nil
}

// Part is the part a preview is read from.
type Part = store.Part

// fetchPreviews reads the first PreviewBytes of each message's text part,
// one FETCH per distinct part path, and renders previews.
func (s *Session) fetchPreviews(ctx context.Context, parts map[uint32]Part) (map[uint32]string, error) {
	byPath := map[string][]uint32{}
	for uid, p := range parts {
		byPath[p.Path] = append(byPath[p.Path], uid)
	}
	out := map[uint32]string{}
	for path, uids := range byPath {
		nums := partNumbers(path)
		partial := &imap.SectionPartial{Offset: 0, Size: PreviewBytes}
		bin := &imap.FetchItemBinarySection{Part: nums, Partial: partial, Peek: true}
		body := &imap.FetchItemBodySection{Part: nums, Partial: partial, Peek: true}
		opts := &imap.FetchOptions{UID: true}
		if s.Caps.Binary {
			opts.BinarySection = []*imap.FetchItemBinarySection{bin}
		} else {
			opts.BodySection = []*imap.FetchItemBodySection{body}
		}
		var msgs []*imapclient.FetchMessageBuffer
		err := s.run(ctx, func() error {
			var err error
			msgs, err = s.c.Fetch(uidSet(uids), opts).Collect()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("fetch previews: %w", err)
		}
		for _, m := range msgs {
			p := parts[uint32(m.UID)]
			encoding := p.Encoding
			var data []byte
			if s.Caps.Binary {
				data, encoding = m.FindBinarySection(bin), "binary"
			} else {
				data = m.FindBodySection(body)
			}
			text, err := mimex.DecodePart(data, encoding, p.Charset, int64(len(data)) >= PreviewBytes)
			if err != nil {
				continue // a broken part only costs the preview
			}
			if p.ContentType == "text/html" {
				text = mimex.HTMLToText(text)
			}
			out[uint32(m.UID)] = mimex.Preview(text, PreviewRunes)
		}
	}
	return out, nil
}

func partNumbers(path string) []int {
	var nums []int
	for f := range strings.SplitSeq(path, ".") {
		n, _ := strconv.Atoi(f)
		nums = append(nums, n)
	}
	return nums
}

// convertHeader maps a header fetch to a MessageHeader and picks the part
// to preview: the first text/plain part that is not an attachment, else the
// first such text/html part.
func convertHeader(m *imapclient.FetchMessageBuffer, extra []byte) (store.MessageHeader, Part) {
	h := store.MessageHeader{
		UID: uint32(m.UID), ModSeq: m.ModSeq, InternalDate: m.InternalDate, Size: m.RFC822Size,
		Flags: store.FlagsFromIMAP(flagStrings(m.Flags)),
	}
	if e := m.Envelope; e != nil {
		h.Subject = e.Subject
		h.Date = e.Date
		if len(e.From) > 0 {
			h.From = address(e.From[0])
		}
		h.To, h.Cc, h.Bcc, h.ReplyTo = addresses(e.To), addresses(e.Cc), addresses(e.Bcc), addresses(e.ReplyTo)
		if ids := mimex.ParseMessageIDs(e.MessageID); len(ids) > 0 {
			h.MessageID = ids[0]
		}
		if ids := mimex.ParseMessageIDs(strings.Join(e.InReplyTo, " ")); len(ids) > 0 {
			h.InReplyTo = ids[0]
		}
	}
	if len(extra) > 0 {
		hdr, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(extra))).ReadMIMEHeader()
		if err == nil || len(hdr) > 0 {
			h.References = mimex.ParseMessageIDs(strings.Join(hdr.Values("References"), " "))
			if ids := mimex.ParseMessageIDs(hdr.Get("List-Id")); len(ids) > 0 {
				h.ListID = ids[0]
			}
			h.ListUnsubscribe = hdr.Get("List-Unsubscribe")
			h.AuthResults = hdr.Get("Authentication-Results")
		}
	}
	var preview, html Part
	if m.BodyStructure != nil {
		m.BodyStructure.Walk(func(path []int, bs imap.BodyStructure) bool {
			sp, ok := bs.(*imap.BodyStructureSinglePart)
			if !ok {
				return true
			}
			p := convertPart(path, sp)
			h.Parts = append(h.Parts, p)
			attached := p.Disposition == "attachment" || (p.Filename != "" && p.Disposition != "inline")
			if attached {
				h.HasAttachments = true
				return true
			}
			switch {
			case p.ContentType == "text/plain" && preview.Path == "":
				preview = p
			case p.ContentType == "text/html" && html.Path == "":
				html = p
			}
			return true
		})
	}
	if preview.Path == "" {
		preview = html
	}
	return h, preview
}

func convertPart(path []int, sp *imap.BodyStructureSinglePart) Part {
	strs := make([]string, len(path))
	for i, n := range path {
		strs[i] = strconv.Itoa(n)
	}
	p := Part{
		Path:        strings.Join(strs, "."),
		ContentType: strings.ToLower(sp.Type + "/" + sp.Subtype),
		Charset:     sp.Params["charset"],
		Encoding:    strings.ToLower(sp.Encoding),
		Filename:    sp.Filename(),
		Size:        int64(sp.Size),
	}
	if ids := mimex.ParseMessageIDs(sp.ID); len(ids) > 0 {
		p.ContentID = ids[0]
	}
	if sp.Extended != nil && sp.Extended.Disposition != nil {
		p.Disposition = strings.ToLower(sp.Extended.Disposition.Value)
	}
	return p
}

func address(a imap.Address) store.Address { return store.Address{Name: a.Name, Addr: a.Addr()} }

func addresses(as []imap.Address) []store.Address {
	var out []store.Address
	for _, a := range as {
		if a.IsGroupStart() || a.IsGroupEnd() {
			continue
		}
		out = append(out, address(a))
	}
	return out
}

func flagStrings(fs []imap.Flag) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = string(f)
	}
	return out
}

// FetchFlags fetches flags for uids in the selected mailbox; with
// changedSince > 0 and CONDSTORE, only those changed since that modseq.
func (s *Session) FetchFlags(ctx context.Context, uids []uint32, changedSince uint64) ([]store.FlagUpdate, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	opts := &imap.FetchOptions{UID: true, Flags: true, ModSeq: s.Caps.CondStore}
	if s.Caps.CondStore {
		opts.ChangedSince = changedSince
	}
	var msgs []*imapclient.FetchMessageBuffer
	err := s.run(ctx, func() error {
		var err error
		msgs, err = s.c.Fetch(uidSet(uids), opts).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fetch flags: %w", err)
	}
	out := make([]store.FlagUpdate, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, store.FlagUpdate{UID: uint32(m.UID), ModSeq: m.ModSeq, Flags: store.FlagsFromIMAP(flagStrings(m.Flags))})
	}
	return out, nil
}

// FetchGmailChanges is FetchFlags plus each message's X-GM-LABELS.
func (s *Session) FetchGmailChanges(ctx context.Context, uids []uint32, changedSince uint64) ([]store.FlagUpdate, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	opts := &imap.FetchOptions{UID: true, Flags: true, ModSeq: s.Caps.CondStore, GmailLabels: true}
	if s.Caps.CondStore {
		opts.ChangedSince = changedSince
	}
	var msgs []*imapclient.FetchMessageBuffer
	err := s.run(ctx, func() error {
		var err error
		msgs, err = s.c.Fetch(uidSet(uids), opts).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fetch gmail changes: %w", err)
	}
	out := make([]store.FlagUpdate, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, store.FlagUpdate{
			UID: uint32(m.UID), ModSeq: m.ModSeq, Flags: store.FlagsFromIMAP(flagStrings(m.Flags)),
			Labels: nonNilLabels(m.GmailLabels),
		})
	}
	return out, nil
}

// nonNilLabels makes "no labels" an empty list, so it differs from "not
// fetched".
func nonNilLabels(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// StoreLabels adds and removes Gmail labels (STORE ±X-GM-LABELS) on uids in
// the selected mailbox.
func (s *Session) StoreLabels(ctx context.Context, uids []uint32, add, remove []string) error {
	if len(uids) == 0 {
		return nil
	}
	err := s.run(ctx, func() error {
		for _, step := range []struct {
			op     imap.StoreFlagsOp
			labels []string
		}{{imap.StoreFlagsAdd, add}, {imap.StoreFlagsDel, remove}} {
			if len(step.labels) == 0 {
				continue
			}
			if err := s.c.StoreGmailLabels(uidSet(uids), step.op, step.labels, true).Close(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("store gmail labels: %w", err)
	}
	return nil
}

// ErrNoMessage means the server no longer has the requested message.
var ErrNoMessage = errors.New("imapx: message not on the server")

// FetchRaw fetches the whole message (BODY.PEEK[]) of uid in the selected
// mailbox.
func (s *Session) FetchRaw(ctx context.Context, uid uint32) ([]byte, error) {
	section := &imap.FetchItemBodySection{Peek: true}
	var msgs []*imapclient.FetchMessageBuffer
	err := s.run(ctx, func() error {
		var err error
		msgs, err = s.c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fetch message %d: %w", uid, err)
	}
	if len(msgs) == 0 {
		return nil, ErrNoMessage
	}
	return msgs[0].FindBodySection(section), nil
}

// StoreFlags adds and removes flags on uids in the selected mailbox.
func (s *Session) StoreFlags(ctx context.Context, uids []uint32, add, remove []string) error {
	for _, op := range []struct {
		kind  imap.StoreFlagsOp
		flags []string
	}{{imap.StoreFlagsAdd, add}, {imap.StoreFlagsDel, remove}} {
		if len(op.flags) == 0 || len(uids) == 0 {
			continue
		}
		fl := make([]imap.Flag, len(op.flags))
		for i, f := range op.flags {
			fl[i] = imap.Flag(f)
		}
		err := s.run(ctx, func() error {
			return s.c.Store(uidSet(uids), &imap.StoreFlags{Op: op.kind, Silent: true, Flags: fl}, nil).Close()
		})
		if err != nil {
			return fmt.Errorf("store flags: %w", err)
		}
	}
	return nil
}

// Move moves uids from the selected mailbox to dest and returns the new UIDs
// by old UID when the server reports them (UIDPLUS). Without MOVE, go-imap
// copies, flags \Deleted and expunges by UID; without UIDPLUS either, the
// move fails rather than risk a bare EXPUNGE.
func (s *Session) Move(ctx context.Context, uids []uint32, dest string) (map[uint32]uint32, error) {
	if !s.Caps.Move && !s.Caps.UIDPlus {
		return nil, errors.New("imapx: server has neither MOVE nor UIDPLUS")
	}
	var d *imapclient.MoveData
	err := s.run(ctx, func() error {
		var err error
		d, err = s.c.Move(uidSet(uids), dest).Wait()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("move to %s: %w", dest, err)
	}
	out := map[uint32]uint32{}
	src, srcOK := d.SourceUIDs.(imap.UIDSet)
	dst, dstOK := d.DestUIDs.(imap.UIDSet)
	if srcOK && dstOK {
		a, b := expand(src), expand(dst)
		for i := range min(len(a), len(b)) {
			out[a[i]] = b[i]
		}
	}
	return out, nil
}

func expand(set imap.UIDSet) []uint32 {
	var out []uint32
	for _, r := range set {
		for u := r.Start; u <= r.Stop && r.Stop != 0; u++ {
			out = append(out, uint32(u))
		}
	}
	return out
}

// Expunge permanently removes uids, which must already be \Deleted, from the
// selected mailbox. It needs UIDPLUS; a bare EXPUNGE could remove messages
// another client marked.
func (s *Session) Expunge(ctx context.Context, uids []uint32) error {
	if !s.Caps.UIDPlus {
		return errors.New("imapx: UID EXPUNGE needs UIDPLUS")
	}
	err := s.run(ctx, func() error { return s.c.UIDExpunge(uidSet(uids)).Close() })
	if err != nil {
		return fmt.Errorf("uid expunge: %w", err)
	}
	return nil
}

// Idle runs IDLE on the selected mailbox until ctx ends, wake fires, or max
// elapses. It returns nil in all three cases unless the connection failed.
// started, if not nil, is called once the server has accepted IDLE.
func (s *Session) Idle(ctx context.Context, wake <-chan struct{}, max time.Duration, started func()) error {
	cmd, err := s.c.Idle()
	if err != nil {
		return fmt.Errorf("idle: %w", err)
	}
	if started != nil {
		started()
	}
	t := time.NewTimer(max)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-wake:
	case <-t.C:
	}
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("idle done: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("idle: %w", err)
	}
	return nil
}

// Append stores a raw message in mailbox with flags and returns its UID when
// the server reports it (UIDPLUS), else 0.
func (s *Session) Append(ctx context.Context, mailbox string, raw []byte, flags []string) (uint32, error) {
	fl := make([]imap.Flag, len(flags))
	for i, f := range flags {
		fl[i] = imap.Flag(f)
	}
	var d *imap.AppendData
	err := s.run(ctx, func() error {
		cmd := s.c.Append(mailbox, int64(len(raw)), &imap.AppendOptions{Flags: fl})
		if _, err := cmd.Write(raw); err != nil {
			return err
		}
		if err := cmd.Close(); err != nil {
			return err
		}
		var err error
		d, err = cmd.Wait()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("append to %s: %w", mailbox, err)
	}
	return uint32(d.UID), nil
}

// Capabilities returns what the server offers, as probed at login.
func (s *Session) Capabilities() Capabilities { return s.Caps }
