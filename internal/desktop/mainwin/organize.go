package mainwin

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// replyItems are the reply family of a row's menu: each answers the row,
// quoting the whole message, as the spec says of the list's menu.
func (w *Window) replyItems(m *ui.Menu, r api.MessageSummary) {
	id := r.ID
	create := func(kind api.DraftKind) {
		w.run("start the message", func(ctx context.Context, c *api.Client) error {
			d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: kind, SourceID: &id})
			if err != nil {
				return err
			}
			w.showDraft(c, d.ID, true)
			return nil
		})
	}
	if m.Item("Reply").Chosen() {
		create(api.DraftKindReply)
	}
	if m.Item("Reply All").Chosen() {
		create(api.DraftKindReplyall)
	}
	if m.Item("Forward").Chosen() {
		create(api.DraftKindForward)
	}
	if m.Item("Forward as Attachment").Chosen() {
		create(api.DraftKindAttached)
	}
	if m.Item("Redirect…").Disabled(w.readOnly(r.AccountID)).Chosen() {
		w.redirect = &redirectSheet{summary: r}
	}
}

// saveSearch opens the smart mailbox sheet on the search shown.
func (w *Window) saveSearch() {
	c, text := w.client, w.q.Search
	if c == nil || text == "" {
		return
	}
	params := &api.SmartFromSearchParams{Text: text}
	if !w.q.SearchAll && w.q.Source.Kind == mail.Mailbox {
		id := w.q.Source.ID
		params.MailboxID = &id
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		conds, err := c.Smart().FromSearch(ctx, params)
		if err != nil {
			w.toast("Could not save the search", err)
			return
		}
		w.o.Post(func() { w.smartSheet.FromSearch(text, *conds) })
	}()
}

// redirectSheet asks whom to redirect a message to.
type redirectSheet struct {
	summary api.MessageSummary
	to      []string
	busy    bool
	failure string
}

func (w *Window) askRedirect() {
	rows := w.rows(w.selectedIDs())
	if len(rows) == 1 && !w.readOnly(rows[0].AccountID) {
		w.redirect = &redirectSheet{summary: rows[0]}
	}
}

func (w *Window) redirectView(c *ui.Context) {
	s := w.redirect
	if s == nil {
		return
	}
	open := true
	good, bad := compose.ParseAddresses(strings.Join(s.to, ","))
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label("Redirect").Children(func() {
			frostui.Heading(c, "Redirect")
			subject := strings.TrimSpace(s.summary.Subject)
			if subject == "" {
				subject = "(no subject)"
			}
			frostui.Muted(c, subject).Wrap()
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Body(c, "To")
				ui.TokenField(c, &s.to, nil).Label("To").Grow(1)
			})
			if s.failure != "" {
				frostui.Toned(c, s.failure, frostui.Danger).Wrap().Role(ui.RoleAlert)
			}
			ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "Redirect").Disabled(len(good) == 0 || len(bad) > 0 || s.busy).Clicked() {
					w.sendRedirect(s, good)
				}
			})
		})
	})
	if !open {
		w.redirect = nil
	}
}

func (w *Window) sendRedirect(s *redirectSheet, to []api.Address) {
	c := w.client
	if c == nil {
		return
	}
	s.busy, s.failure = true, ""
	id := s.summary.ID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		item, err := c.Message().Redirect(ctx, &api.MessageRedirectParams{ID: id, To: to})
		w.o.Post(func() {
			s.busy = false
			if err != nil {
				s.failure = err.Error()
				return
			}
			w.redirect = nil
			w.Sent(item)
		})
	}()
}
