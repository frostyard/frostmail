package mainwin

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// downloadTimeout bounds fetching an attachment.
const downloadTimeout = 120 * time.Second

type liveState = live.MailState

func base64Decode(s string) []byte {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	return data
}

func decodePhoto(data []byte) *ui.Bitmap {
	b, err := uikit.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	return b
}

func copyFile(from, to string) error {
	in, err := os.Open(from) //nolint:gosec // a part of the cache
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(to) //nolint:gosec // the file the user chose
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (w *Window) rawSourceCommand() {
	if m, ok := w.expandSelected(); ok {
		w.showRawSource(w.conv, m)
	}
}

func (w *Window) allHeadersCommand() {
	if m, ok := w.expandSelected(); ok {
		w.toggleHeaders(w.conv, m)
	}
}

// OpenMessage shows a message, from a notification, a link or the
// palette, once a connection can look it up.
func (w *Window) OpenMessage(id int64) {
	w.pendingOpen = id
	w.deliverOpen()
}

func (w *Window) deliverOpen() {
	id, c := w.pendingOpen, w.client
	if id == 0 || c == nil {
		return
	}
	st := w.mail.State()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		got, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{id}})
		if err != nil {
			return // the next connection delivers it
		}
		w.o.Post(func() {
			if w.pendingOpen != id {
				return
			}
			w.pendingOpen = 0
			if len(got) > 0 {
				w.reveal(got[0], st)
			}
		})
	}()
}

// ReplyTo starts a reply to a message in a compose window, from a
// notification's Reply (parity P-805). A request made before there is a
// connection waits for one. Once draft.create is sent the request is
// spent, whatever comes back: unlike OpenMessage's lookup, the call makes
// a draft each time, and one that ends with the connection may have made
// it, so it is never sent again.
func (w *Window) ReplyTo(id int64) {
	w.pendingReply = id
	w.deliverReply()
}

func (w *Window) deliverReply() {
	id, c := w.pendingReply, w.client
	if id == 0 || c == nil {
		return
	}
	w.pendingReply = 0 // one click sends one draft.create
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &id})
		if err != nil {
			slog.Warn("reply from a notification", "message", id, "err", err)
			return
		}
		w.showDraft(c, d.ID, true)
	}()
}

// reveal shows a message in its account's inbox, or its first mailbox, and
// selects it; a list filter that would hide it goes.
func (w *Window) reveal(s api.MessageSummary, st liveState) {
	source := mail.Source{Kind: mail.AllInboxes}
	for _, m := range st.Mailboxes {
		if m.AccountID == s.AccountID && m.Role == api.MailboxRoleInbox {
			for _, id := range s.MailboxIDs {
				if id == m.ID {
					source = mail.Source{Kind: mail.Mailbox, ID: m.ID}
				}
			}
		}
	}
	if source.Kind == mail.AllInboxes && len(s.MailboxIDs) > 0 {
		source = mail.Source{Kind: mail.Mailbox, ID: s.MailboxIDs[0]}
	}
	w.q.Filter = mail.FilterAll
	w.setSource(source)
	w.chosen.Clear()
	w.chosen.Add(s.ID)
	w.selected = -1
	w.revealing = &s
}

// syncReveal moves the list's selection onto a revealed row once it loads.
func (w *Window) syncReveal() {
	if w.revealing == nil || w.view == nil {
		return
	}
	if i := w.view.IndexOf(w.revealing.ID); i >= 0 && w.selected != int(i) {
		w.selected = int(i)
		w.lastIndex = i
		w.list.ScrollIntoView(int(i))
	}
}

// selectionForReply reads the selection a reply quotes: the message it
// belongs to and the text (ADR-0053). The page of the message focused comes
// first, then the others in the reader's order: a page keeps its selection
// when the focus leaves it, so the first in order can be an older one. It
// waits for the pages: call it from a worker.
func selectionForReply(pages []pageOf, focused int64) (int64, string) {
	if i := slices.IndexFunc(pages, func(p pageOf) bool { return p.id == focused }); i > 0 {
		pages = slices.Concat(pages[i:i+1], pages[:i], pages[i+1:])
	}
	for _, p := range pages {
		if s := p.frame.Selection(); strings.TrimSpace(s) != "" {
			return p.id, s
		}
	}
	return 0, ""
}

// reply starts a reply to the selected message, quoting the text selected
// in its conversation if any.
func (w *Window) reply(kind api.DraftKind) {
	ids := w.selectedIDs()
	if len(ids) != 1 {
		return
	}
	pages := w.frames()
	var focused int64
	if w.conv != nil {
		focused = w.conv.focused
	}
	id := ids[0]
	w.run("start the reply", func(ctx context.Context, c *api.Client) error {
		source := id
		params := &api.DraftCreateParams{Kind: kind}
		if from, quote := selectionForReply(pages, focused); from != 0 {
			source, params.Quote = from, &quote
		}
		params.SourceID = &source
		d, err := c.Draft().Create(ctx, params)
		if err != nil {
			return err
		}
		w.showDraft(c, d.ID, true)
		return nil
	})
}

func (w *Window) forward() {
	ids := w.selectedIDs()
	if len(ids) != 1 {
		return
	}
	id := ids[0]
	w.run("start the forward", func(ctx context.Context, c *api.Client) error {
		d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindForward, SourceID: &id})
		if err != nil {
			return err
		}
		w.showDraft(c, d.ID, true)
		return nil
	})
}

func (w *Window) compose() {
	w.run("start a message", func(ctx context.Context, c *api.Client) error {
		d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew})
		if err != nil {
			return err
		}
		w.showDraft(c, d.ID, true)
		return nil
	})
}

func (w *Window) composeTo(address, name string) {
	w.run("start a message", func(ctx context.Context, c *api.Client) error {
		d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, To: []api.Address{{Name: name, Address: address}}})
		if err != nil {
			return err
		}
		w.showDraft(c, d.ID, true)
		return nil
	})
}

// showDraft opens a draft's window, on the UI thread.
func (w *Window) showDraft(c *api.Client, draft int64, fresh bool) {
	if w.o.OpenCompose == nil {
		return
	}
	w.o.Post(func() { w.o.OpenCompose(c, draft, fresh) })
}

// openSelected opens a message in a Drafts mailbox in a compose window.
func (w *Window) openSelected() {
	ids := w.selectedIDs()
	rows := w.rows(ids)
	if len(rows) != 1 || !mail.InRole(rows[0], api.MailboxRoleDrafts, w.mail.State().Mailboxes) {
		return
	}
	id := rows[0].ID
	w.run("open the draft", func(ctx context.Context, c *api.Client) error {
		d, err := c.Draft().Open(ctx, &api.DraftOpenParams{MessageID: id})
		if err != nil {
			return err
		}
		w.showDraft(c, d.ID, false)
		return nil
	})
}

// Sent shows the undo toast of a message that went into the Outbox. A
// message sent later has none: it waits in its own section.
func (w *Window) Sent(item *api.OutboxItem) {
	if item.Scheduled || item.SendAt == nil {
		return
	}
	left := item.SendAt.Sub(w.o.Now())
	if left <= 0 {
		return
	}
	subject := strings.TrimSpace(item.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	id := item.ID
	w.onFrame = append(w.onFrame, func(c *ui.Context) {
		frostui.ShowActionToast(c, frostui.ActionToast{Title: "Sending “" + subject + "”…", Action: "Undo", Timeout: left, Countdown: true,
			OnAction: w.bound(func() { w.undoSend(id) })})
	})
}

// undoSend cancels a send and reopens its draft.
func (w *Window) undoSend(id int64) {
	w.run("undo the send", func(ctx context.Context, c *api.Client) error {
		d, err := c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: id})
		if err != nil {
			return err
		}
		w.showDraft(c, d.ID, false)
		return nil
	})
}

// findState is the find bar's: the matches of each page shown.
type findState struct {
	open    bool
	query   string
	asked   string
	current int
	moved   int
	counts  []int
	pages   []pageOf
	token   int
}

func (w *Window) startFind() {
	if w.selectedMessage() == nil {
		w.focusSearch = true
		return
	}
	w.find.open = true
}

func (w *Window) closeFind() {
	if !w.find.open {
		return
	}
	for _, p := range w.find.pages {
		p.frame.StopFind()
	}
	// The token goes on from here: a Find still running must not match
	// the next one's.
	w.find = findState{token: w.find.token + 1}
}

func (w *Window) totalMatches() int {
	total := 0
	for _, n := range w.find.counts {
		total += n
	}
	return total
}

// findBar draws the bar and carries out its moves.
func (w *Window) findBar(c *ui.Context) {
	f := &w.find
	if !f.open {
		return
	}
	bar := ui.FindBar(c, &f.open, &f.query, w.totalMatches(), &f.current)
	if !f.open {
		query := f.query
		w.find.open = true
		w.closeFind()
		w.find.query = query
		return
	}
	if f.query != f.asked {
		w.runFind(f.query)
		return
	}
	if bar.Changed() {
		w.stepFind(f.current)
	}
}

// runFind asks every page for its matches of text, and shows the first.
func (w *Window) runFind(text string) {
	f := &w.find
	f.asked, f.current, f.moved = text, 0, 0
	f.token++
	token := f.token
	pages := w.frames()
	f.pages = pages
	go func() {
		counts := make([]int, len(pages))
		for i, p := range pages {
			if text == "" {
				p.frame.StopFind()
				continue
			}
			_, counts[i] = p.frame.Find(text, false, false)
		}
		w.o.Post(func() {
			if w.find.token == token {
				w.find.counts = counts
			}
		})
	}()
}

// stepFind moves to match target, across the pages: within a page by
// next or previous, and to a new page by finding from its start.
func (w *Window) stepFind(target int) {
	f := &w.find
	from := f.moved
	f.moved = target
	pages, counts, text := f.pages, f.counts, f.query
	if len(counts) != len(pages) {
		return // the counts are of other pages, until this find's arrive
	}
	go func() {
		page, local := locate(counts, target)
		fromPage, fromLocal := locate(counts, from)
		if page < 0 {
			return
		}
		if page != fromPage {
			_, _ = pages[page].frame.Find(text, false, false)
			fromLocal = 0
		}
		for fromLocal < local {
			_, _ = pages[page].frame.Find(text, true, false)
			fromLocal++
		}
		for fromLocal > local {
			_, _ = pages[page].frame.Find(text, true, true)
			fromLocal--
		}
	}()
}

// locate turns a match's index across the pages into its page and its
// index there; -1 for none.
func locate(counts []int, index int) (page, local int) {
	for i, n := range counts {
		if index < n {
			return i, index
		}
		index -= n
	}
	return -1, 0
}

// showPerson shows a person in People.
func (w *Window) showPerson(id int64) {
	w.setModule(commands.People)
	w.pim.ShowPerson(id)
}

// showOccurrence shows an event in Calendar.
func (w *Window) showOccurrence(o api.Occurrence) {
	w.setModule(commands.Calendar)
	w.pim.ShowOccurrence(o)
}

// showInvitation shows an invitation's event in Calendar.
func (w *Window) showInvitation(inv *api.Invitation) {
	e := inv.Event
	o := api.Occurrence{EventID: e.ID, Summary: e.Summary, AllDay: e.AllDay, Start: e.Start, End: e.End, StartDate: e.StartDate, EndDate: e.EndDate}
	if inv.EventID != nil {
		o.EventID = *inv.EventID
	}
	w.showOccurrence(o)
}

func (w *Window) askRemind(ids []int64, at time.Time) {
	w.later.Show("Remind Me", at, func(chosen time.Time) { w.remind(ids, &chosen) })
}
