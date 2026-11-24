package main

import (
	"context"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type command struct {
	label, group, key string
	enabled           func() bool
	run               func()
}

// A command's shortcut and its palette row call the same action.
func (w *workspace) commandTable() []command {
	mailMode := func() bool { return w.module.GetUntracked() == "Mail" }
	hasMail := func() bool { return mailMode() && w.reader.GetUntracked() != nil }
	message := func(f func(*messagePage)) func() {
		return func() {
			if p := w.reader.GetUntracked(); p != nil {
				f(p)
			}
		}
	}
	draft := func(kind api.DraftKind) func() {
		return message(func(p *messagePage) { w.newDraft(kind, &p.message.Summary.ID) })
	}
	active := func() bool {
		m := w.currentMessage.GetUntracked()
		return hasMail() && m != nil && w.writable(m.AccountID)
	}
	triage := func(f func(api.MessageSummary)) func() {
		return func() {
			if m := w.currentMessage.GetUntracked(); m != nil {
				f(*m)
			}
		}
	}
	flag := func(m api.MessageSummary, changes api.FlagChanges) {
		undo := w.undo
		w.change(func(ctx context.Context, c *api.Client) error { return undo.Flags(ctx, c, []int64{m.ID}, changes) }, nil)
	}
	commands := []command{
		{"Compose", "Mail", "mod+n", mailMode, func() { w.newDraft(api.DraftKindNew, nil) }},
		{"Open drafts", "Mail", "", nil, w.draftList}, {"Open outbox", "Mail", "", nil, w.outbox},
		{"Reply", "Message", "mod+r", hasMail, draft(api.DraftKindReply)}, {"Reply all", "Message", "mod+shift+r", hasMail, draft(api.DraftKindReplyall)},
		{"Forward", "Message", "mod+shift+f", hasMail, draft(api.DraftKindForward)}, {"Forward as attachment", "Message", "", hasMail, draft(api.DraftKindAttached)},
		{"Redirect", "Message", "mod+shift+e", active, triage(w.redirect)},
		{"Toggle flag", "Message", "mod+shift+l", active, triage(func(m api.MessageSummary) { v := !m.Flags.Flagged; flag(m, api.FlagChanges{Flagged: &v}) })},
		{"Toggle read", "Message", "mod+shift+u", active, triage(func(m api.MessageSummary) { v := !m.Flags.Seen; flag(m, api.FlagChanges{Seen: &v}) })},
		{"Delete message", "Message", "delete", active, triage(w.deleteMessage)},
		{"Archive", "Message", "mod+alt+a", active, triage(func(m api.MessageSummary) {
			if box := mail.ArchiveOf(m.AccountID, w.store.State().Mailboxes); box != nil {
				w.change(w.moveMessage(m, *box), w.moved("Message moved to "+box.Name))
			}
		})},
		{"Toggle junk", "Message", "mod+shift+j", active, triage(func(m api.MessageSummary) {
			if _, box := mail.SpamTarget([]api.MessageSummary{m}, w.store.State().Mailboxes); box != nil {
				w.change(w.moveMessage(m, *box), w.moved("Message moved to "+box.Name))
			}
		})},
		{"Print or save PDF", "Message", "mod+p", hasMail, message(w.printMessage)},
		{"All headers", "Message", "mod+shift+h", hasMail, message(func(p *messagePage) { w.sourcePanel(p.message.Summary, true) })},
		{"Raw source", "Message", "mod+alt+u", hasMail, message(func(p *messagePage) { w.sourcePanel(p.message.Summary, false) })},
		{"Save message as", "Message", "", hasMail, message(func(p *messagePage) { w.saveMessage(p.message.Summary) })},
		{"Search messages", "Mail", "mod+alt+f", mailMode, func() { w.searchInput.Focus() }},
		{"Undo", "Mail", "mod+z", mailMode, func() {
			undo := w.undo
			w.change(func(ctx context.Context, c *api.Client) error { _, err := undo.Do(ctx, c); return err }, nil)
		}},
		{"All inboxes", "Workspace", "mod+1", nil, func() { w.navigate("all-inboxes") }},
		{"Sync accounts", "Mail", "mod+shift+n", nil, func() {
			accounts := w.store.State().Accounts
			w.change(func(ctx context.Context, c *api.Client) error {
				for _, a := range accounts {
					if err := c.Sync().Now(ctx, &api.SyncNowParams{AccountID: a.ID}); err != nil {
						return err
					}
				}
				return nil
			}, nil)
		}},
	}
	find := ""
	if desktopServer {
		find = "mod+f"
	}
	commands = append(commands, command{"Find or quote message text", "Message", find, hasMail, message(w.textReader)})
	for _, target := range []struct{ module, key string }{{"Calendar", "mod+2"}, {"People", "mod+3"}, {"Tasks", "mod+4"}, {"Settings", "mod+,"}, {"Activity", ""}} {
		commands = append(commands, command{target.module, "Workspace", target.key, nil, func() { w.navigate("module:" + target.module) }})
	}
	return commands
}

func (w *workspace) palette() {
	if w.overlay.GetUntracked() != nil {
		return
	}
	table := w.commandTable()
	w.show(func(cx *core.Scope) core.Node {
		run := map[string]func(){}
		var rows []base.CommandItem
		for _, c := range table {
			if c.enabled != nil && !c.enabled() {
				continue
			}
			run[c.label] = c.run
			rows = append(rows, base.CommandItem{ID: c.label, Label: c.label, Group: c.group, Shortcut: c.key})
		}
		for _, section := range w.sections() {
			for _, item := range section.Items {
				if strings.HasPrefix(item.ID, "module:") || strings.HasPrefix(item.ID, "panel:") {
					continue
				}
				key := "navigate/" + item.ID
				run[key] = func() { w.navigate(item.ID) }
				rows = append(rows, base.CommandItem{ID: key, Label: item.Text, Group: section.Title})
			}
		}
		return ui.CommandDialog{Style: core.Style{Background: workspaceui.ColorsOf(cx).Panel()}, Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				w.closePanel()
			}
		}, Command: ui.Command{Label: "Commands", Placeholder: "Find a command or mailbox", AutoFocus: true, Items: func() []base.CommandItem { return rows }, OnSelect: func(id string) {
			if f := run[id]; f != nil {
				w.closePanel()
				f()
			}
		}}}
	})
}
