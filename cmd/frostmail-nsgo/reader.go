package main

import (
	"context"
	"fmt"
	"github.com/frostyard/frostmail/api"
	oldcompose "github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"net/url"
	"slices"
	"strings"
	"time"
)

func (w *workspace) sourcePanel(m api.MessageSummary, headers bool) {
	perform(w, func(ctx context.Context, c *api.Client) (*api.MessageSource, error) {
		return c.Message().Source(ctx, &api.MessageSourceParams{ID: m.ID})
	}, func(s *api.MessageSource, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		title, text := "Raw source", s.Text
		if headers {
			title, text = "All headers", s.Headers
		} else if s.Truncated {
			text += "\n… (preview truncated)"
		}
		w.show(func(*core.Scope) core.Node {
			return w.dialog(title, []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(440)}, Children: []core.Node{core.Text{Content: text, Selectable: core.SelectableOn}}}}, button("Save message as", func() { w.saveMessage(m) }), button("Close", w.closePanel))
		})
	})
}
func (w *workspace) senderActions(m api.MessageSummary) []core.Node {
	st := w.store.State()
	blocked, vip := false, false
	for _, b := range st.Blocked {
		if strings.EqualFold(b.Address, m.From.Address) {
			blocked = true
			break
		}
	}
	for _, v := range st.VIPs {
		if strings.EqualFold(v.Address, m.From.Address) {
			vip = true
			break
		}
	}
	mailAction := func(text string, fn func(context.Context, *api.Client) error) core.Node {
		return ui.MenuItem{Text: text, OnSelect: func() { w.change(fn, nil) }}
	}
	addresses := []string{m.From.Address}
	blabel := "Block sender"
	if blocked {
		blabel = "Unblock sender"
	}
	vlabel := "Add sender to VIPs"
	if vip {
		vlabel = "Remove sender from VIPs"
	}
	return []core.Node{mailAction("Add sender to Contacts", func(ctx context.Context, c *api.Client) error {
		_, err := c.People().Add(ctx, &api.PeopleAddParams{Email: m.From.Address, Name: changed("", m.From.Name)})
		return err
	}), mailAction(blabel, func(ctx context.Context, c *api.Client) error {
		if blocked {
			return c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: addresses})
		}
		_, err := c.Block().Add(ctx, &api.BlockAddParams{Addresses: addresses})
		return err
	}), mailAction(vlabel, func(ctx context.Context, c *api.Client) error {
		if vip {
			return c.Vip().Remove(ctx, &api.VipRemoveParams{Addresses: addresses})
		}
		_, err := c.Vip().Add(ctx, &api.VipAddParams{Addresses: addresses})
		return err
	})}
}
func (w *workspace) moreMessageActions(m api.MessageSummary) []core.Node {
	return []core.Node{ui.Submenu{Text: "Sender", Children: w.senderActions(m)}, ui.MenuItem{Text: "Show headers", OnSelect: func() { w.sourcePanel(m, true) }}, ui.MenuItem{Text: "Raw source", OnSelect: func() { w.sourcePanel(m, false) }}, ui.MenuItem{Text: "Save message as", OnSelect: func() { w.saveMessage(m) }}, ui.MenuItem{Text: "Apply mail rules", OnSelect: func() {
		w.change(func(ctx context.Context, c *api.Client) error {
			_, err := c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: []int64{m.ID}})
			return err
		}, nil)
	}}, ui.MenuItem{Text: "Redirect", OnSelect: func() {
		w.form("Redirect message", []editField{{"Recipients", "", false}}, func(v []string) error {
			tos, invalid := oldcompose.ParseAddresses(v[0])
			if len(invalid) > 0 || len(tos) == 0 {
				return fmt.Errorf("Enter a valid recipient")
			}
			w.confirm("Redirect message?", m.Subject+" → "+v[0], "Send", func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					_, err := c.Message().Redirect(ctx, &api.MessageRedirectParams{ID: m.ID, To: tos})
					return err
				}, nil)
			})
			return nil
		})
	}}}
}
func (w *workspace) subscription(id int64) {
	perform(w, func(ctx context.Context, c *api.Client) (*api.Unsubscribe, error) {
		return c.Message().UnsubscribeInfo(ctx, &api.MessageUnsubscribeInfoParams{ID: id})
	}, func(s *api.Unsubscribe, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.show(func(*core.Scope) core.Node {
			nodes := []core.Node{core.Text{Content: s.List}}
			for _, m := range s.Methods {
				nodes = append(nodes, button(mail.UnsubscribeSentence(s, m), func() {
					w.confirm("Unsubscribe?", s.List, "Unsubscribe", func() {
						perform(w, func(ctx context.Context, c *api.Client) (*api.UnsubscribeResult, error) {
							return c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: id, Method: m})
						}, func(r *api.UnsubscribeResult, err error) {
							if err != nil {
								w.problem.Set(err.Error())
								return
							}
							if r.URL != nil {
								u, err := url.Parse(*r.URL)
								if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
									w.cx.OpenURL(*r.URL, nil)
								}
							}
						})
					})
				}))
			}
			return w.dialog("Mailing list", nodes)
		})
	})
}
func (w *workspace) invitation(id int64) {
	perform(w, func(ctx context.Context, c *api.Client) (*api.Invitation, error) {
		return c.Calendar().Invitation(ctx, &api.CalendarInvitationParams{MessageID: id})
	}, func(inv *api.Invitation, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.show(func(*core.Scope) core.Node {
			info := mail.InvitationLines(inv, time.Local)
			nodes := []core.Node{core.Text{Content: inv.Event.Summary, Style: ui.TypeTitle}, core.Text{Content: info.When}, core.Text{Content: info.Who}, core.Text{Content: strings.TrimSpace(info.Conflicts + " " + info.Adjacent + " " + info.Status)}}
			if inv.CanRespond {
				var comment string
				nodes = append(nodes, stringField("Note to organizer", &comment, false))
				for _, answer := range []api.PartStat{api.PartStatAccepted, api.PartStatTentative, api.PartStatDeclined} {
					nodes = append(nodes, button(string(answer), func() {
						p := api.CalendarRespondParams{MessageID: &id, Answer: answer, Comment: changed("", comment)}
						w.confirm("Send invitation response?", inv.Event.Summary+" · "+string(answer), "Send", func() {
							w.change(func(ctx context.Context, c *api.Client) error { _, err := c.Calendar().Respond(ctx, &p); return err }, nil)
						})
					}))
				}
			}
			if inv.CanAdd {
				nodes = append(nodes, button("Add to calendar", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Calendar().AddFromMessage(ctx, &api.CalendarAddFromMessageParams{MessageID: id})
						return err
					}, w.closePanel)
				}))
			}
			return w.dialog("Calendar invitation", nodes)
		})
	})
}
func (w *workspace) readerExtras(m *api.Message) []core.Node {
	var nodes []core.Node
	invitation := mail.InvitationPart(m.Parts) != nil
	if invitation {
		nodes = append(nodes, button("Calendar invitation", func() { w.invitation(m.Summary.ID) }))
	}
	if m.ListUnsubscribe != "" {
		nodes = append(nodes, button("Unsubscribe from list", func() { w.subscription(m.Summary.ID) }))
	}
	parts := mail.Attachments(m.Parts, invitation)
	if len(parts) > 0 {
		nodes = append(nodes, workspaceui.Section{Title: "Attachments", Count: func() int { return len(parts) }, Content: func(*core.Scope) core.Node {
			var files []core.Node
			for _, p := range parts {
				name := p.Filename
				if name == "" {
					name = p.ContentType
				}
				files = append(files, button("Save "+name+" · "+mail.FormatSize(p.Size), func() { w.savePart(m.Summary.ID, p) }))
			}
			return workspaceui.Stack(files...)
		}})
	}
	return nodes
}

func (w *workspace) writable(accountID int64) bool {
	for _, a := range w.store.State().Accounts {
		if a.ID == accountID {
			return !a.ReadOnly
		}
	}
	return false
}

// Update the reader toolbar on a server-side change without replacing the
// body WebView, discarding its scroll position or re-reading unread mail.
func (w *workspace) watchReader() {
	var revision int
	w.cx.Cleanup(w.events.On(func(event api.Event) {
		switch v := event.(type) {
		case api.MessageRemoved:
			w.cx.Post(func() {
				if slices.Contains(v.IDs, w.opened.GetUntracked()) {
					w.generation++
					w.opened.Set(0)
					w.reader.Set(nil)
				}
			})
		case api.MessageChanged:
			w.cx.Post(func() {
				id := w.opened.GetUntracked()
				if id == 0 || !slices.Contains(v.IDs, id) {
					return
				}
				revision++
				ticket, gen := revision, w.generation
				perform(w, func(ctx context.Context, c *api.Client) ([]api.MessageSummary, error) {
					return c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{id}})
				}, func(rows []api.MessageSummary, err error) {
					if ticket == revision && gen == w.generation && err == nil && len(rows) == 1 {
						w.currentMessage.Set(&rows[0])
					}
				})
			})
		}
	}))
}
func (w *workspace) remind(id int64, at *time.Time) {
	w.change(func(ctx context.Context, c *api.Client) error {
		return c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{id}, At: at})
	}, w.closePanel)
}
func (w *workspace) remindMenu(m api.MessageSummary) ui.Submenu {
	var items []core.Node
	for _, choice := range mail.RemindChoices(time.Now()) {
		items = append(items, ui.MenuItem{Text: choice.Label, OnSelect: func() { w.remind(m.ID, &choice.At) }})
	}
	items = append(items, ui.MenuItem{Text: "Choose reminder time", OnSelect: func() {
		const layout = "2006-01-02 15:04"
		w.form("Remind me", []editField{{"Local date and time", time.Now().Add(time.Hour).Format(layout), false}}, func(v []string) error {
			at, err := time.ParseInLocation(layout, v[0], time.Local)
			if err != nil || !at.After(time.Now()) {
				return fmt.Errorf("Enter a future date and time as YYYY-MM-DD HH:MM")
			}
			w.remind(m.ID, &at)
			return nil
		})
	}})
	if m.RemindAt != nil {
		items = append(items, ui.MenuItem{Text: "Cancel reminder", OnSelect: func() { w.remind(m.ID, nil) }})
	}
	return ui.Submenu{Text: "Remind me", Children: items}
}
