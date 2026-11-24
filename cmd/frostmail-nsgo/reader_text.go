package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The selectable text comes from maild, independently of a page's
// origin. Searching or quoting never gives an email script privileges.
func (w *workspace) textReader(page *messagePage) {
	id := page.message.Summary.ID
	text := page.rendering.Text
	w.show(func(cx *core.Scope) core.Node {
		needle := core.NewSignal(cx, "")
		selection := core.NewSignal(cx, "")
		preview := core.Dynamic(func(*core.Scope) core.Node {
			query := strings.ToLower(strings.TrimSpace(needle.Get()))
			visible := text
			if query != "" {
				var matching []string
				for _, line := range strings.Split(text, "\n") {
					if strings.Contains(strings.ToLower(line), query) {
						matching = append(matching, line)
					}
				}
				if len(matching) == 0 {
					return core.Text{Content: "No matching text"}
				}
				visible = strings.Join(matching, "\n")
			}
			return core.View{Selectable: true, OnTextSelection: func(s core.TextSelection) { selection.Set(s.Text) }, Children: []core.Node{core.Text{Content: visible}}}
		})
		reply := func(kind api.DraftKind) {
			quote := selection.GetUntracked()
			if strings.TrimSpace(quote) == "" {
				return
			}
			perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
				return c.Draft().Create(ctx, &api.DraftCreateParams{Kind: kind, SourceID: &id, Quote: &quote})
			}, w.editDraft)
		}
		disabled := func() bool { return strings.TrimSpace(selection.Get()) == "" }
		return w.dialog("Find or quote message text", []core.Node{ui.Input{Label: "Find in message", Placeholder: "Find in message", Value: needle.Get, OnChange: func(v string) { selection.Set(""); needle.Set(v) }}, core.Scroll{Style: core.Style{MaxHeight: core.Pt(410)}, Children: []core.Node{preview}}}, button("Close", w.closePanel), ui.Button{Text: "Reply with selection", Disabled: disabled, OnPress: func() { reply(api.DraftKindReply) }}, ui.Button{Text: "Reply all with selection", Disabled: disabled, OnPress: func() { reply(api.DraftKindReplyall) }})
	})
}

func printHeaders(m *api.Message) []string {
	summary := m.Summary
	header := []string{"From: " + addressLine([]api.Address{summary.From}), "To: " + addressLine(m.To)}
	if len(m.Cc) > 0 {
		header = append(header, "Cc: "+addressLine(m.Cc))
	}
	header = append(header, "Date: "+summary.Date.Local().Format("Mon, 02 Jan 2006 15:04 MST"))
	if m.SenderCheck != nil && *m.SenderCheck == api.SenderCheckFail {
		header = append(header, "Sender verification failed")
	}
	return header
}

func (w *workspace) readerTools(id int64) []core.Node {
	do := func(f func(*messagePage)) func() {
		return func() {
			if p := w.reader.GetUntracked(); p != nil && p.message.Summary.ID == id {
				f(p)
			}
		}
	}
	return []core.Node{ui.MenuItem{Text: "Find or quote message text", OnSelect: do(w.textReader)}, ui.MenuItem{Text: "Print or save PDF", OnSelect: do(w.printMessage)}}
}

func (w *workspace) contactCard(address string) {
	perform(w, func(ctx context.Context, c *api.Client) (*api.ContactCard, error) {
		return c.People().Card(ctx, &api.PeopleCardParams{Email: address})
	}, func(card *api.ContactCard, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.show(func(cx *core.Scope) core.Node {
			nodes := []core.Node{core.Text{Content: card.Name, Style: ui.TypeTitle}, core.Text{Content: card.Email, Selectable: core.SelectableOn}}
			if card.Person != nil {
				if card.Person.HasPhoto {
					nodes = append(nodes, w.contactPhoto(cx, card.Person.ID))
				}
				nodes = append(nodes, button("Open contact", func() { w.personDetail(card.Person, func() {}) }))
			} else if card.CanAdd {
				nodes = append(nodes, button("Add to Contacts", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.People().Add(ctx, &api.PeopleAddParams{Email: card.Email, Name: changed("", card.Name)})
						return err
					}, func() { w.contactCard(address) })
				}))
			}
			for _, m := range card.Recent {
				nodes = append(nodes, button(m.Subject, func() { w.closePanel(); w.module.Set("Mail"); w.openMessage(m.ID, false) }))
			}
			for _, o := range card.Upcoming {
				nodes = append(nodes, button(fmt.Sprintf("%s · %s", o.Summary, o.Start.Local().Format("Jan 2")), func() {
					w.applyRoute(eventPath(api.CalendarEvent{ID: o.EventID, RecurrenceID: o.RecurrenceID}), func() {})
				}))
			}
			return w.dialog("Sender", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(435)}, Children: []core.Node{workspaceui.Stack(nodes...)}}})
		})
	})
}
