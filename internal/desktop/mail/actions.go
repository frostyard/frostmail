package mail

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

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

// MoveGroup is one account's share of a selection, and where it goes: a
// message moves only within its own account.
type MoveGroup struct {
	IDs []int64
	To  api.Mailbox
}

// byAccount splits rows by account, in the order the accounts first come.
func byAccount(rows []api.MessageSummary) [][]api.MessageSummary {
	var out [][]api.MessageSummary
	at := map[int64]int{}
	for _, r := range rows {
		i, ok := at[r.AccountID]
		if !ok {
			i = len(out)
			at[r.AccountID] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], r)
	}
	return out
}

func idsOf(rows []api.MessageSummary) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// ArchiveGroups sends each account's messages to its own archive, and
// leaves out an account that has none.
func ArchiveGroups(rows []api.MessageSummary, mailboxes []api.Mailbox) []MoveGroup {
	var out []MoveGroup
	for _, group := range byAccount(rows) {
		if to := ArchiveOf(group[0].AccountID, mailboxes); to != nil {
			out = append(out, MoveGroup{IDs: idsOf(group), To: *to})
		}
	}
	return out
}

// SpamGroups sends each account's messages to its own inbox when every row
// is in its account's Junk, else to its own Junk, and leaves out an account
// without that mailbox.
func SpamGroups(rows []api.MessageSummary, mailboxes []api.Mailbox) (notSpam bool, groups []MoveGroup) {
	notSpam = len(rows) > 0 && !slices.ContainsFunc(rows, func(r api.MessageSummary) bool {
		junk := withRole(r.AccountID, api.MailboxRoleJunk, mailboxes)
		return junk == nil || !slices.Contains(r.MailboxIDs, junk.ID)
	})
	role := api.MailboxRoleJunk
	if notSpam {
		role = api.MailboxRoleInbox
	}
	for _, group := range byAccount(rows) {
		if to := withRole(group[0].AccountID, role, mailboxes); to != nil {
			groups = append(groups, MoveGroup{IDs: idsOf(group), To: *to})
		}
	}
	return notSpam, groups
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

// How an undo waits for a move maild has not replayed yet.
const (
	undoRetry    = 200 * time.Millisecond
	undoPatience = 10 * time.Second
)

// ErrUndoWaiting says an undo found its messages still moving on the
// server; its entry stays, for another try.
var ErrUndoWaiting = errors.New("the messages are still moving on the server; try Undo again in a moment")

// Undo keeps inverse actions in memory, oldest first. One Undo belongs to
// one maild: its calls name that maild's messages and mailboxes.
type Undo struct {
	// Retry and Patience are how often and how long an undo tries again
	// while its messages are still moving [200ms, 10s].
	Retry, Patience time.Duration

	mu      sync.Mutex
	entries [][]UndoCall
	// acting is held for reading while an action runs, and for writing
	// while an undo does. A row leaves the list before its action has
	// remembered its inverse, so Do waits; and inverses run one at a time,
	// newest first.
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

// ErrNotRemembered says an action was done, but could not be read back, so
// Undo cannot take it back.
var ErrNotRemembered = errors.New("done, but Undo cannot take it back")

// Move runs act, a move or a delete of ids, and remembers the inverse of
// what it changed: also when act fails part way, as when one account's
// messages moved and another's did not. It returns the messages as maild
// holds them after act, an erased one not among them, and nil when they
// could not be read. Its error is act's, or ErrNotRemembered when act was
// done but could not be read back.
func (u *Undo) Move(ctx context.Context, c *api.Client, ids []int64, act func() error) ([]api.MessageSummary, error) {
	if len(ids) == 0 {
		return []api.MessageSummary{}, nil
	}
	u.acting.RLock()
	defer u.acting.RUnlock()
	params := &api.MessageSummariesParams{IDs: ids}
	before, err := c.Message().Summaries(ctx, params)
	if err != nil {
		return nil, err
	}
	acted := act()
	after, err := c.Message().Summaries(ctx, params)
	switch {
	case err != nil && acted != nil:
		return nil, acted
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrNotRemembered, err)
	}
	u.remember(MoveBack(before, after))
	if after == nil {
		after = []api.MessageSummary{}
	}
	return after, acted
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

// Do performs the newest inverse action and removes it; false when none.
// It waits for an action still running, which is the newest once it ends,
// and for an undo still running. An entry whose messages are still moving
// stays (ErrUndoWaiting); no action can add one above it meanwhile, as
// actions wait for an undo.
func (u *Undo) Do(ctx context.Context, c *api.Client) (bool, error) {
	u.acting.Lock()
	defer u.acting.Unlock()
	u.mu.Lock()
	if len(u.entries) == 0 {
		u.mu.Unlock()
		return false, nil
	}
	entry := u.entries[len(u.entries)-1]
	u.mu.Unlock()
	err := u.perform(ctx, c, entry)
	if errors.Is(err, ErrUndoWaiting) {
		return true, err
	}
	u.mu.Lock()
	u.entries = u.entries[:len(u.entries)-1]
	u.mu.Unlock()
	return true, err
}

func (u *Undo) perform(ctx context.Context, c *api.Client, entry []UndoCall) error {
	for _, call := range entry {
		var err error
		switch {
		case call.Move != nil:
			err = u.untilLanded(ctx, c, call.Move.IDs, call.Move.MailboxID, func() error { return c.Message().Move(ctx, call.Move) })
		case call.Copy != nil:
			err = u.untilLanded(ctx, c, call.Copy.IDs, call.Copy.MailboxID, func() error { return c.Message().Copy(ctx, call.Copy) })
		default:
			err = c.Message().SetFlags(ctx, call.Flags)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// untilLanded runs call, a move or copy of ids into mailbox, until it takes
// effect. maild skips, and reports as done, a message whose earlier move it
// has not replayed on the server yet: that call changes nothing, and an
// undo soon after its action would be lost.
func (u *Undo) untilLanded(ctx context.Context, c *api.Client, ids []int64, mailbox int64, call func() error) error {
	retry, patience := u.Retry, u.Patience
	if retry == 0 {
		retry = undoRetry
	}
	if patience == 0 {
		patience = undoPatience
	}
	deadline := time.Now().Add(patience)
	params := &api.MessageSummariesParams{IDs: ids}
	for {
		before, err := c.Message().Summaries(ctx, params)
		if err != nil {
			return err
		}
		if err := call(); err != nil {
			return err
		}
		after, err := c.Message().Summaries(ctx, params)
		if err != nil {
			return err
		}
		if !skipped(before, after, mailbox) {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrUndoWaiting
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retry):
		}
	}
}

// skipped reports whether a message that still exists is outside mailbox
// with the memberships it had before the call.
func skipped(before, after []api.MessageSummary, mailbox int64) bool {
	was := make(map[int64][]int64, len(before))
	for _, r := range before {
		was[r.ID] = r.MailboxIDs
	}
	return slices.ContainsFunc(after, func(r api.MessageSummary) bool {
		return !slices.Contains(r.MailboxIDs, mailbox) && slices.Equal(r.MailboxIDs, was[r.ID])
	})
}
