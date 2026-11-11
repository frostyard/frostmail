package mail

import (
	"context"
	"encoding/json/v2"
	"slices"
	"strconv"
	"sync"

	"github.com/frostyard/frostmail/api"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// SourceHolds reports whether a source still lists a message: its mailbox
// for a mailbox source, an inbox for All Inboxes, and anything else for a
// source that is not one mailbox (docs/specs/ui.md, Follow Up).
func SourceHolds(s Source, m api.MessageSummary, mailboxes []api.Mailbox) bool {
	switch s.Kind {
	case Mailbox:
		return slices.Contains(m.MailboxIDs, s.ID)
	case AllInboxes:
		return slices.ContainsFunc(m.MailboxIDs, func(id int64) bool {
			return slices.ContainsFunc(mailboxes, func(mb api.Mailbox) bool { return mb.ID == id && mb.Role == api.MailboxRoleInbox })
		})
	}
	return true
}

// ArchiveOf is where Archive moves an account's messages: its archive
// mailbox, or All Mail on an account whose mailboxes are Gmail labels.
func ArchiveOf(account int64, mailboxes []api.Mailbox) *api.Mailbox {
	var all *api.Mailbox
	labels := false
	for i := range mailboxes {
		m := &mailboxes[i]
		if m.AccountID != account {
			continue
		}
		if m.Role == api.MailboxRoleArchive {
			return m
		}
		labels = labels || m.Label
		if m.Role == api.MailboxRoleAll {
			all = m
		}
	}
	if labels {
		return all
	}
	return nil
}

func withRole(account int64, role api.MailboxRole, mailboxes []api.Mailbox) *api.Mailbox {
	for i := range mailboxes {
		if mailboxes[i].AccountID == account && mailboxes[i].Role == role {
			return &mailboxes[i]
		}
	}
	return nil
}

// SpamTarget chooses Junk, or the inbox when every row is in Junk.
func SpamTarget(rows []api.MessageSummary, mailboxes []api.Mailbox) (notSpam bool, to *api.Mailbox) {
	if len(rows) == 0 {
		return false, nil
	}
	junk := withRole(rows[0].AccountID, api.MailboxRoleJunk, mailboxes)
	notSpam = junk != nil && !slices.ContainsFunc(rows, func(r api.MessageSummary) bool { return !slices.Contains(r.MailboxIDs, junk.ID) })
	if notSpam {
		return true, withRole(rows[0].AccountID, api.MailboxRoleInbox, mailboxes)
	}
	return false, junk
}

// MoveTargets lists the message's account's mailboxes that do not hold it.
func MoveTargets(m *api.MessageSummary, mailboxes []api.Mailbox) []api.Mailbox {
	if m == nil {
		return nil
	}
	var out []api.Mailbox
	for _, mb := range mailboxes {
		if mb.AccountID == m.AccountID && !slices.Contains(m.MailboxIDs, mb.ID) {
			out = append(out, mb)
		}
	}
	return out
}

// CopyTargets limits Gmail destinations to labels other than Starred.
func CopyTargets(m *api.MessageSummary, mailboxes []api.Mailbox) []api.Mailbox {
	targets := MoveTargets(m, mailboxes)
	if m == nil || !slices.ContainsFunc(mailboxes, func(mb api.Mailbox) bool { return mb.AccountID == m.AccountID && mb.Label }) {
		return targets
	}
	return slices.DeleteFunc(targets, func(mb api.Mailbox) bool { return !mb.Label || mb.Role == api.MailboxRoleFlagged })
}

// MoveParams are message.move's params. When the list shows a mailbox of
// the destination's account (or a unified one), the messages leave that
// one: on Gmail, moving out of a label removes only that label.
func MoveParams(ids []int64, to api.Mailbox, s Source, mailboxes []api.Mailbox) *api.MessageMoveParams {
	p := &api.MessageMoveParams{IDs: ids, MailboxID: to.ID}
	switch s.Kind {
	case Mailbox:
		for _, mb := range mailboxes {
			if mb.ID == s.ID && mb.AccountID == to.AccountID {
				p.FromMailboxID = &mb.ID
			}
		}
	case Role:
		if from := withRole(to.AccountID, s.Role, mailboxes); from != nil {
			p.FromMailboxID = &from.ID
		}
	}
	return p
}

// UndoLimit is how many actions the main window remembers.
const UndoLimit = 20

// UndoCall restores a message's earlier mailbox membership or flags.
type UndoCall struct {
	Move  *api.MessageMoveParams
	Copy  *api.MessageCopyParams
	Flags *api.MessageSetFlagsParams
}

func mergeCalls(calls []UndoCall) []UndoCall {
	var out []UndoCall
	keys := map[string]int{}
	for _, c := range calls {
		var key string
		var ids *[]int64
		var one []int64
		switch {
		case c.Move != nil:
			m := *c.Move
			one, m.IDs = m.IDs, nil
			b, _ := json.Marshal(m)
			key = "move" + string(b)
		case c.Copy != nil:
			m := *c.Copy
			one, m.IDs = m.IDs, nil
			b, _ := json.Marshal(m)
			key = "copy" + string(b)
		default:
			m := *c.Flags
			one, m.IDs = m.IDs, nil
			b, _ := json.Marshal(m)
			key = "flags" + string(b)
		}
		i, ok := keys[key]
		if !ok {
			keys[key] = len(out)
			out = append(out, c)
			continue
		}
		switch {
		case out[i].Move != nil:
			ids = &out[i].Move.IDs
		case out[i].Copy != nil:
			ids = &out[i].Copy.IDs
		default:
			ids = &out[i].Flags.IDs
		}
		*ids = append(*ids, one...)
	}
	return out
}

// MoveBack restores the memberships each surviving message left.
func MoveBack(before, after []api.MessageSummary) []UndoCall {
	current := make(map[int64]api.MessageSummary, len(after))
	for _, r := range after {
		current[r.ID] = r
	}
	var moves, copies []UndoCall
	for _, r := range before {
		next, ok := current[r.ID]
		if !ok {
			continue
		}
		var left, entered []int64
		for _, id := range r.MailboxIDs {
			if !slices.Contains(next.MailboxIDs, id) {
				left = append(left, id)
			}
		}
		for _, id := range next.MailboxIDs {
			if !slices.Contains(r.MailboxIDs, id) {
				entered = append(entered, id)
			}
		}
		if len(left) == 0 {
			continue
		}
		rest := left
		if len(entered) > 0 {
			from := entered[0]
			moves = append(moves, UndoCall{Move: &api.MessageMoveParams{IDs: []int64{r.ID}, MailboxID: left[0], FromMailboxID: &from}})
			rest = left[1:]
		}
		for _, id := range rest {
			copies = append(copies, UndoCall{Copy: &api.MessageCopyParams{IDs: []int64{r.ID}, MailboxID: id}})
		}
	}
	return mergeCalls(append(moves, copies...))
}

// FlagsBack restores the earlier read marks and flag colors that changed.
func FlagsBack(before []api.MessageSummary, c api.FlagChanges) []UndoCall {
	var calls []UndoCall
	for _, r := range before {
		if c.Seen != nil && *c.Seen != r.Flags.Seen {
			seen := r.Flags.Seen
			calls = append(calls, UndoCall{Flags: &api.MessageSetFlagsParams{IDs: []int64{r.ID}, Changes: api.FlagChanges{Seen: &seen}}})
		}
		color := int64(0)
		if r.Flags.Flagged {
			color = r.Flags.FlagColor
		}
		if c.FlagColor != nil && *c.FlagColor != color {
			calls = append(calls, UndoCall{Flags: &api.MessageSetFlagsParams{IDs: []int64{r.ID}, Changes: api.FlagChanges{FlagColor: &color}}})
		}
	}
	return mergeCalls(calls)
}

// Undo keeps inverse actions in memory, oldest first.
type Undo struct {
	mu      sync.Mutex
	entries [][]UndoCall
	// acting is held for reading while an action runs. A row leaves the
	// list before its action has remembered its inverse, so Do waits.
	acting sync.RWMutex
}

func (u *Undo) remember(calls []UndoCall) {
	if len(calls) == 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.entries = append(u.entries, calls)
	if len(u.entries) > UndoLimit {
		u.entries = u.entries[len(u.entries)-UndoLimit:]
	}
}

// Len is how many actions can be undone.
func (u *Undo) Len() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.entries)
}

// Move runs act, a move or a delete of ids, and remembers its inverse.
func (u *Undo) Move(ctx context.Context, c *api.Client, ids []int64, act func() error) error {
	if len(ids) == 0 {
		return nil
	}
	u.acting.RLock()
	defer u.acting.RUnlock()
	before, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		return err
	}
	if err := act(); err != nil {
		return err
	}
	after, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		return err
	}
	u.remember(MoveBack(before, after))
	return nil
}

// Flags changes flags and remembers the earlier values.
func (u *Undo) Flags(ctx context.Context, c *api.Client, ids []int64, ch api.FlagChanges) error {
	if len(ids) == 0 {
		return nil
	}
	u.acting.RLock()
	defer u.acting.RUnlock()
	before, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		return err
	}
	if err := c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: ids, Changes: ch}); err != nil {
		return err
	}
	u.remember(FlagsBack(before, ch))
	return nil
}

// Do removes and performs the newest inverse action; false when none. It
// waits for an action still running, which is the newest once it ends.
func (u *Undo) Do(ctx context.Context, c *api.Client) (bool, error) {
	u.acting.Lock()
	u.acting.Unlock() //nolint:staticcheck // only waits for the actions running
	u.mu.Lock()
	if len(u.entries) == 0 {
		u.mu.Unlock()
		return false, nil
	}
	entry := u.entries[len(u.entries)-1]
	u.entries = u.entries[:len(u.entries)-1]
	u.mu.Unlock()
	for _, call := range entry {
		var err error
		switch {
		case call.Move != nil:
			err = c.Message().Move(ctx, call.Move)
		case call.Copy != nil:
			err = c.Message().Copy(ctx, call.Copy)
		default:
			err = c.Message().SetFlags(ctx, call.Flags)
		}
		if err != nil {
			return true, err
		}
	}
	return true, nil
}
