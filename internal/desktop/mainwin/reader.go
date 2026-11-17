package mainwin

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// maxConversation is how many of a conversation's messages show before
// Show N Earlier Messages (docs/specs/ui.md, Reader).
const maxConversation = 20

// conv is the conversation the reader shows. It belongs to the UI thread.
//
// maild answers calls in any order, so an answer can be older than a row
// the conversation holds. Each message.changed gets the next sequence
// number; a call carries the number of the last event it saw when it was
// asked, and a row is replaced only by an answer that is not older than
// the row's.
type conv struct {
	id       int64
	client   *api.Client
	ctx      context.Context
	cancel   context.CancelFunc
	rows     []api.MessageSummary // newest first
	loaded   bool
	err      error
	expanded map[int64]bool
	showAll  bool
	msgs     map[int64]*openMsg
	focused  int64 // the message whose page last had the focus
	// seq counts the message.changed events since the conversation opened.
	// held is, for each row in rows, the seq its data was asked at; changed
	// is the seq of the last event of each message, kept while the load,
	// asked before it, is out.
	seq     uint64
	held    map[int64]uint64
	changed map[int64]uint64
}

// openMsg is one expanded message: what the reader draws for it.
type openMsg struct {
	summary       api.MessageSummary
	message       *api.Message
	rendering     *api.Rendering
	err           error
	loadingRemote bool
	headers       *string
	frame         uikit.Frame
	printOnLoad   bool
	invitation    *api.Invitation
	inviteBusy    bool
	unsub         *api.Unsubscribe
	unsubBusy     bool
	unsubFailed   bool
	contact       *contactCard
	// thumbs are the thumbnails of picture attachments, by part path, as
	// they are made (docs/specs/ui.md, Attachments).
	thumbs map[string]*ui.Bitmap
	// pageScale is the text size the page was loaded at (mail.TextScale).
	pageScale float32
}

// syncReader follows the selection: exactly one message shows, its
// conversation loaded once.
func (w *Window) syncReader() {
	ids := w.selectedIDs()
	var want int64
	if len(ids) == 1 {
		want = ids[0]
	}
	if w.conv != nil && w.conv.id == want && w.conv.client == w.client {
		return
	}
	w.closeFind() // before the pages it searches close
	w.closeConv()
	if want != 0 && w.client != nil {
		w.openConv(want)
	}
}

func (w *Window) closeConv() {
	if w.conv == nil {
		return
	}
	w.conv.cancel()
	for _, m := range w.conv.msgs {
		w.dropFrame(m)
	}
	w.conv = nil
}

func (w *Window) dropFrame(m *openMsg) {
	if m.frame != nil {
		m.frame.Close()
		m.frame = nil
	}
}

func (w *Window) openConv(id int64) {
	ctx, cancel := context.WithCancel(context.Background())
	cv := &conv{id: id, client: w.client, ctx: ctx, cancel: cancel, expanded: map[int64]bool{id: true}, msgs: map[int64]*openMsg{},
		held: map[int64]uint64{}, changed: map[int64]uint64{}}
	w.conv = cv
	st := w.mail.State()
	asked := cv.seq
	go func() {
		rows, err := loadConversation(ctx, cv.client, id, st)
		w.o.Post(func() {
			if w.conv != cv {
				return
			}
			again := cv.store(rows, err, asked)
			w.conversationLoaded(cv, st)
			w.askSummaries(cv, again, cv.seq)
		})
	}()
}

// store puts the rows the load answered, asked at seq, in the conversation
// and returns the messages among them that changed after seq: the load may
// have read them before that change. The kept events are dropped then:
// the call that reads those rows again carries the current seq.
func (cv *conv) store(rows []api.MessageSummary, err error, asked uint64) (again []int64) {
	cv.rows, cv.err, cv.loaded = rows, err, true
	cv.held = make(map[int64]uint64, len(rows))
	for _, r := range rows {
		cv.held[r.ID] = asked
		if cv.changed[r.ID] > asked {
			again = append(again, r.ID)
		}
	}
	cv.changed = nil
	return again
}

// loadConversation lists the selected message's conversation, newest
// first, leaving out what is in Trash unless the selected message is.
func loadConversation(ctx context.Context, c *api.Client, id int64, st stateMailboxes) ([]api.MessageSummary, error) {
	got, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{id}})
	if err != nil {
		return nil, err
	}
	if len(got) == 0 {
		return nil, fmt.Errorf("this message is gone")
	}
	s := got[0]
	rows := []api.MessageSummary{s}
	if s.ThreadID != 0 {
		if rows, err = c.Thread().Messages(ctx, &api.ThreadMessagesParams{ID: s.ThreadID}); err != nil {
			return nil, err
		}
	}
	return filterConversation(rows, s, id, st), nil
}

type stateMailboxes = liveState

func filterConversation(rows []api.MessageSummary, s api.MessageSummary, id int64, st liveState) []api.MessageSummary {
	trash := map[int64]bool{}
	for _, m := range st.Mailboxes {
		if m.Role == api.MailboxRoleTrash {
			trash[m.ID] = true
		}
	}
	inTrash := func(r api.MessageSummary) bool {
		return slices.ContainsFunc(r.MailboxIDs, func(m int64) bool { return trash[m] })
	}
	out := slices.DeleteFunc(slices.Clone(rows), func(r api.MessageSummary) bool { return r.ID != id && inTrash(r) && !inTrash(s) })
	slices.Reverse(out)
	return out
}

// conversationLoaded opens what was unread, and marks what shows read.
func (w *Window) conversationLoaded(cv *conv, st liveState) {
	if cv.err != nil {
		return
	}
	var unseen []int64
	for i, r := range cv.rows {
		if r.Flags.Seen {
			continue
		}
		cv.expanded[r.ID] = true
		if i < maxConversation && !w.readOnly(r.AccountID) {
			unseen = append(unseen, r.ID)
		}
	}
	if len(unseen) > 0 {
		seen := true
		w.run("mark the message read", func(ctx context.Context, c *api.Client) error {
			return c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: unseen, Changes: api.FlagChanges{Seen: &seen}})
		})
	}
	w.syncExpanded(cv)
}

// shownRows are the rows the reader draws.
func (cv *conv) shownRows() []api.MessageSummary {
	if cv.showAll || len(cv.rows) <= maxConversation {
		return cv.rows
	}
	return cv.rows[:maxConversation]
}

// isOpen reports whether a row draws expanded: a conversation of one
// always does.
func (cv *conv) isOpen(id int64) bool { return len(cv.rows) <= 1 || cv.expanded[id] }

// syncExpanded loads each expanded message and drops the pages of those
// collapsed: a page exists only for a message that shows it.
func (w *Window) syncExpanded(cv *conv) {
	for _, r := range cv.shownRows() {
		if !cv.isOpen(r.ID) {
			if m := cv.msgs[r.ID]; m != nil {
				w.dropFrame(m)
				delete(cv.msgs, r.ID)
			}
			continue
		}
		if cv.msgs[r.ID] == nil {
			w.loadMessage(cv, r)
		}
	}
}

func (w *Window) setExpanded(cv *conv, id int64, open bool) {
	cv.expanded[id] = open
	w.closeFind() // before a collapsed message's page closes
	w.syncExpanded(cv)
}

func (w *Window) loadMessage(cv *conv, r api.MessageSummary) {
	m := &openMsg{summary: r}
	cv.msgs[r.ID] = m
	asked := cv.seq
	go func() {
		ctx := cv.ctx
		msg, err := cv.client.Message().Get(ctx, &api.MessageGetParams{ID: r.ID})
		var rendering *api.Rendering
		if err == nil {
			rendering, err = cv.client.Message().Render(ctx, &api.MessageRenderParams{ID: r.ID})
		}
		w.o.Post(func() {
			if w.conv != cv || cv.msgs[r.ID] != m {
				return
			}
			m.message, m.err = msg, err
			if err != nil {
				return
			}
			// A refresh asked after this call is newer than its summary.
			cv.take([]api.MessageSummary{msg.Summary}, asked)
			w.showRendering(m, rendering)
			w.loadExtras(cv, m)
		})
	}()
}

// showRendering puts a rendering in the message's page, making it the
// first time.
func (w *Window) showRendering(m *openMsg, r *api.Rendering) {
	m.rendering = r
	if m.frame == nil && w.o.NewFrame != nil {
		f, err := w.o.NewFrame()
		if err != nil {
			m.err = err
			return
		}
		m.frame = f
	}
	if m.frame == nil {
		return
	}
	body, style := r.HTML, ""
	if body == "" {
		body, style = mail.TextHTML(r.Text), mail.TextStyle
	}
	m.pageScale = w.currentTextScale()
	m.frame.Load(uikit.Document(body, w.printHeader(m), mail.PrintGuard+mail.ZoomStyle(m.pageScale)+style))
	if m.printOnLoad {
		m.printOnLoad = false
		m.frame.Print()
	}
}

// currentTextScale is the text size the window holds now (mail.TextScale);
// the usual one before it holds any.
func (w *Window) currentTextScale() float32 {
	if w.mail == nil {
		return 1
	}
	return mail.TextScale(w.mail.State().Settings)
}

// followTextSize runs every frame. When the text size the window holds
// has changed, the pages of the open messages load again at the new size
// (the page zooms; its height follows from the measure), and the app's
// other windows draw again. Each page loads again once at a size, so a
// frame that changes nothing makes no call.
func (w *Window) followTextSize() {
	scale := w.currentTextScale()
	if cv := w.conv; cv != nil {
		stale := func(m *openMsg) bool { return m.frame != nil && m.rendering != nil && m.pageScale != scale }
		if slices.ContainsFunc(slices.Collect(maps.Values(cv.msgs)), stale) {
			// A page that loads again has lost its highlights and its
			// current match, and a search can not wait for the load: the
			// find bar closes, as it does when a page closes, and the
			// next search starts from the first match of the new pages.
			w.closeFind()
		}
		for _, m := range cv.msgs {
			if stale(m) {
				w.showRendering(m, m.rendering)
			}
		}
	}
	if scale != w.textScale {
		w.textScale = scale
		if w.o.TextSizeChanged != nil {
			w.o.TextSizeChanged()
		}
	}
}

func (w *Window) printHeader(m *openMsg) string {
	if m.message == nil {
		return ""
	}
	addresses := func(list []api.Address) string {
		parts := make([]string, 0, len(list))
		for _, a := range list {
			parts = append(parts, mail.DisplayName(a))
		}
		return strings.Join(parts, ", ")
	}
	s := m.message.Summary
	warning := ""
	if m.message.SenderCheck != nil && *m.message.SenderCheck == api.SenderCheckFail {
		warning = mail.SenderCheckText(s.From.Address)
	}
	return mail.PrintHeader(mail.DisplayName(s.From), addresses(m.message.To), addresses(m.message.Cc), s.Date.In(w.o.Now().Location()).Format("January 2, 2006 at 3:04 PM"), s.Subject, warning)
}

// loadExtras asks for what only some messages have: an invitation, and how
// to leave a mailing list.
func (w *Window) loadExtras(cv *conv, m *openMsg) {
	w.loadThumbnails(cv, m)
	if mail.InvitationPart(m.message.Parts) != nil {
		w.loadInvitation(cv, m)
	}
	if m.message.ListUnsubscribe != "" {
		id := m.summary.ID
		go func() {
			info, err := cv.client.Message().UnsubscribeInfo(cv.ctx, &api.MessageUnsubscribeInfoParams{ID: id})
			if err != nil {
				return
			}
			w.o.Post(func() { m.unsub = info })
		}()
	}
}

func (w *Window) loadInvitation(cv *conv, m *openMsg) {
	id := m.summary.ID
	go func() {
		inv, err := cv.client.Calendar().Invitation(cv.ctx, &api.CalendarInvitationParams{MessageID: id})
		w.o.Post(func() {
			if err != nil {
				m.invitation = nil
				return
			}
			m.invitation = inv
		})
	}()
}

func (w *Window) loadRemote(cv *conv, m *openMsg) {
	m.loadingRemote = true
	remote := true
	id := m.summary.ID
	go func() {
		r, err := cv.client.Message().Render(cv.ctx, &api.MessageRenderParams{ID: id, Remote: &remote})
		w.o.Post(func() { w.remoteRendered(cv, m, r, err) })
	}()
}

// remoteRendered shows a rendering with its remote content, if the message
// still shows: one collapsed meanwhile has no page, and gets none.
func (w *Window) remoteRendered(cv *conv, m *openMsg, r *api.Rendering, err error) {
	m.loadingRemote = false
	if err == nil && w.conv == cv && cv.msgs[m.summary.ID] == m {
		w.showRendering(m, r)
	}
}

// readerEvent keeps the reader current; it runs on the events' goroutine.
func (w *Window) readerEvent(ev api.Event) {
	switch e := ev.(type) {
	case api.MessageChanged:
		w.o.Post(func() { w.refreshSummaries(e.IDs) })
	case api.CalendarChanged:
		w.o.Post(func() {
			if cv := w.conv; cv != nil {
				for _, m := range cv.msgs {
					if m.message != nil && mail.InvitationPart(m.message.Parts) != nil {
						w.loadInvitation(cv, m)
					}
				}
			}
		})
	}
}

// refreshSummaries re-reads the rows a message.changed named. Every event
// is counted, whether or not its message is a row yet: the load of a
// conversation was asked before it.
func (w *Window) refreshSummaries(ids []int64) {
	cv := w.conv
	if cv == nil {
		return
	}
	seq := cv.messageChanged(ids)
	var want []int64
	for _, r := range cv.rows {
		if slices.Contains(ids, r.ID) {
			want = append(want, r.ID)
		}
	}
	w.askSummaries(cv, want, seq)
}

// messageChanged counts a message.changed of ids and returns its seq. A
// load in flight was asked before it, so the event is kept for the load's
// rows to see.
func (cv *conv) messageChanged(ids []int64) uint64 {
	cv.seq++
	if !cv.loaded {
		if cv.changed == nil {
			cv.changed = map[int64]uint64{}
		}
		for _, id := range ids {
			cv.changed[id] = cv.seq
		}
	}
	return cv.seq
}

// askSummaries asks for the rows of ids, at seq, the message.changed event
// that named them, and posts their answer to the conversation.
func (w *Window) askSummaries(cv *conv, ids []int64, seq uint64) {
	if len(ids) == 0 {
		return
	}
	go func() {
		fresh, err := cv.client.Message().Summaries(cv.ctx, &api.MessageSummariesParams{IDs: ids})
		if err != nil {
			return
		}
		w.o.Post(func() {
			if w.conv != cv {
				return
			}
			cv.take(fresh, seq)
		})
	}()
}

// take puts message.summaries' rows, asked at seq, in place of the rows of
// the same messages, and reports whether one changed. A row asked after
// seq is newer and stays: two answers can come in any order.
func (cv *conv) take(fresh []api.MessageSummary, seq uint64) bool {
	var changed bool
	for _, s := range fresh {
		if cv.held[s.ID] > seq {
			continue
		}
		found := false
		for i, r := range cv.rows {
			if r.ID == s.ID {
				cv.rows[i] = s
				found = true
			}
		}
		if found {
			if cv.held == nil {
				cv.held = map[int64]uint64{}
			}
			cv.held[s.ID] = seq
			changed = true
		}
		if m := cv.msgs[s.ID]; m != nil {
			m.summary = s
			changed = true
		}
	}
	return changed
}

// reader draws the reader pane.
func (w *Window) reader(c *ui.Context) {
	w.syncReader()
	ids := w.selectedIDs()
	ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(c.Theme().Background).Children(func() {
		w.readerActions(c)
		w.findBar(c)
		switch {
		case len(ids) == 0:
			frostui.Empty(c, "mail-read", "No Message Selected", "").Grow(1)
		case len(ids) > 1:
			frostui.Empty(c, "mail-read", fmt.Sprintf("%d Messages Selected", len(ids)), "").Grow(1)
		case w.conv == nil || !w.conv.loaded:
			ui.Box(c).Grow(1)
		case w.conv.err != nil:
			frostui.Empty(c, "dialog-warning", w.conv.err.Error(), "").Grow(1)
		default:
			w.conversation(c, w.conv)
		}
	})
}

func (w *Window) conversation(c *ui.Context, cv *conv) {
	scroll := ui.Scroll(c).Grow(1).Fill().Label("Message").Focusable().TrackScroll(&w.readerScroll)
	w.readerHeight = scroll.LayoutBounds().H
	w.readerFocused = scroll.FocusWithin()
	if w.focusPane == paneReader {
		w.focusPane = ""
		scroll.Focus()
	}
	if w.readerFocused {
		w.pane = paneReader
	}
	scroll.Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			shown := cv.shownRows()
			for i, r := range shown {
				if i > 0 {
					frostui.RowDivider(c.Key([2]any{"divider", r.ID}))
				}
				if cv.isOpen(r.ID) {
					if m := cv.msgs[r.ID]; m != nil {
						w.message(c.Key(r.ID), cv, m)
					}
				} else {
					w.collapsed(c.Key(r.ID), cv, r)
				}
			}
			if n := len(cv.rows) - len(shown); n > 0 {
				if ui.Button(c, fmt.Sprintf("Show %d Earlier Messages", n)).Margin(frostui.SpaceM).Clicked() {
					cv.showAll = true
					w.syncExpanded(cv)
				}
			}
		})
	})
}

// collapsed draws a message of a conversation as one row.
func (w *Window) collapsed(c *ui.Context, cv *conv, r api.MessageSummary) {
	t := c.Theme()
	row := ui.ButtonBase(c).Row().Gap(frostui.SpaceM).Padding(frostui.SpaceM, frostui.SpaceL).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(mail.DisplayName(r.From) + ": " + r.Subject)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() {
		w.setExpanded(cv, r.ID, true)
	}
	row.Children(func() {
		ui.Avatar(c, mail.DisplayName(r.From), nil).Size(24, 24).Radius(t.Radius)
		frostui.Strong(c, mail.DisplayName(r.From)).SingleLine()
		frostui.Muted(c, r.Preview).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		frostui.Caption(c, mail.ListDate(r.Date, w.o.Now())).SingleLine()
	})
}

// message draws an expanded message: its header, banners, page and
// attachments.
func (w *Window) message(c *ui.Context, cv *conv, m *openMsg) {
	ui.Column(c).AlignItems(ui.Stretch).Label(m.summary.Subject).Children(func() {
		if m.message != nil {
			w.header(c, cv, m)
			w.banners(c, cv, m)
		}
		if m.headers != nil {
			ui.Column(c).Padding(0, frostui.SpaceL, frostui.SpaceL, frostui.SpaceL).AlignItems(ui.Stretch).Label("All Headers").Children(func() {
				frostui.Mono(c, *m.headers).Wrap()
			})
		}
		if m.err != nil {
			frostui.Muted(c, m.err.Error()).Wrap().Padding(frostui.SpaceL)
		}
		if m.frame != nil {
			if uikit.PageFrame(c, m.frame, max(m.frame.Height(), 40)).Label("Message body").FocusWithin() {
				cv.focused = m.summary.ID
			}
		}
		if m.message != nil {
			w.attachments(c, cv, m)
		}
	})
}

// selectedMessage is the message the commands act on, when it shows.
func (w *Window) selectedMessage() *openMsg {
	if w.conv == nil {
		return nil
	}
	return w.conv.msgs[w.conv.id]
}

// expandSelected opens the selected message if it is collapsed; ok once
// its state exists. Source and headers can load alongside its body.
func (w *Window) expandSelected() (*openMsg, bool) {
	if w.conv == nil {
		return nil, false
	}
	if !w.conv.isOpen(w.conv.id) {
		w.setExpanded(w.conv, w.conv.id, true)
	}
	m := w.selectedMessage()
	return m, m != nil
}

// frames are the pages shown, in the reader's order.
func (w *Window) frames() []pageOf {
	if w.conv == nil {
		return nil
	}
	var out []pageOf
	for _, r := range w.conv.shownRows() {
		if m := w.conv.msgs[r.ID]; m != nil && m.frame != nil {
			out = append(out, pageOf{id: r.ID, frame: m.frame})
		}
	}
	return out
}

type pageOf struct {
	id    int64
	frame uikit.Frame
}

func (w *Window) printMessage() {
	if m, ok := w.expandSelected(); ok {
		if m.frame != nil {
			m.frame.Print()
		} else {
			m.printOnLoad = true
		}
	}
}

// pageStep is how far Space scrolls the reader, as a part of its height.
const pageStep = 0.9

// pageReader scrolls the reader by nine tenths of its height.
func (w *Window) pageReader(direction float32) {
	w.readerScroll.Y += direction * pageStep * w.readerHeight
}

// cyclePane moves the keyboard to the next pane, or the previous one.
func (w *Window) cyclePane(step int) {
	panes := []string{paneList, paneReader}
	if w.shell.Panes.Shown(paneSidebar) {
		panes = []string{paneSidebar, paneList, paneReader}
	}
	i := slices.Index(panes, w.pane)
	if i < 0 {
		i = 0
	}
	w.focusPane = panes[(i+step+len(panes))%len(panes)]
	w.pane = w.focusPane
}

func (w *Window) timeLabel(t time.Time) string {
	return t.In(w.o.Now().Location()).Format("Mon, Jan 2, 2006 at 3:04 PM")
}
