package mainwin

import (
	"context"
	"strconv"
	"strings"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// The palette's sources besides the commands.
const (
	sourceMailboxes = "mailboxes"
	sourceMessages  = "messages"

	mailboxResults = 6
	messageResults = 8
)

func (w *Window) paletteSources() []frostui.PaletteSource {
	return []frostui.PaletteSource{
		{ID: sourceMailboxes, Title: "Mailboxes", Placeholder: "Find a mailbox", Limit: mailboxResults, Query: w.queryMailboxes},
		{ID: sourceMessages, Title: "Messages", Placeholder: "Find a message", Limit: messageResults, Query: w.queryMessages},
	}
}

// queryMailboxes finds the sidebar's rows by name. Like a message's, an
// item names one maild's mailbox: chosen after a change of maild, it does
// nothing.
func (w *Window) queryMailboxes(q *frostui.PaletteQuery) {
	if q.Text == "" {
		q.Reply(nil)
		return
	}
	var rows []mail.Item
	for _, it := range w.tree {
		if it.Selectable {
			rows = append(rows, it)
		}
	}
	st := w.mail.State()
	var items []frostui.PaletteItem
	for _, it := range frostui.Rank(rows, q.Text, 0, func(it mail.Item) string { return it.Label }) {
		source, ok := mail.SourceFromKey(it.Key, st.VIPs)
		if !ok {
			continue
		}
		items = append(items, frostui.PaletteItem{ID: it.Key, Title: it.Label, Icon: it.Icon, Run: w.bound(func() { w.setModule(commands.Mail); w.setSource(source) })})
	}
	q.Reply(items)
}

// queryMessages searches every mailbox through a view of its own, on a
// worker, and drops the answer if the user has typed on, or if the window
// has moved to another maild: its IDs name the last one's mail, and so an
// item chosen after the move does nothing.
func (w *Window) queryMessages(q *frostui.PaletteQuery) {
	text, c := strings.TrimSpace(q.Text), w.client
	if text == "" || c == nil {
		q.Reply(nil)
		return
	}
	query := mail.ListQuery(mail.ListState{Source: mail.Source{Kind: mail.AllInboxes}, Search: text, SearchAll: true,
		Filter: mail.FilterAll, Sort: api.ViewSortDate})
	var rows []api.MessageSummary
	reply := w.bound(func() {
		items := make([]frostui.PaletteItem, 0, len(rows))
		for _, r := range rows {
			id := r.ID
			items = append(items, frostui.PaletteItem{ID: strconv.FormatInt(id, 10), Title: r.Subject, Detail: mail.DisplayName(r.From),
				Icon: "mail-read", Run: w.bound(func() { w.showMessage(id) })})
		}
		q.Reply(items)
	})
	go func() {
		var err error
		rows, err = searchMessages(q.Context(), c, query)
		if err != nil {
			return
		}
		w.o.Post(reply)
	}()
}

func searchMessages(ctx context.Context, c *api.Client, query api.ViewQuery) ([]api.MessageSummary, error) {
	info, err := c.View().Open(ctx, &api.ViewOpenParams{Query: query})
	if err != nil {
		return nil, err
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		_ = c.View().Close(cctx, &api.ViewCloseParams{ID: info.ID})
	}()
	return c.View().Range(ctx, &api.ViewRangeParams{ID: info.ID, Start: 0, End: messageResults})
}
