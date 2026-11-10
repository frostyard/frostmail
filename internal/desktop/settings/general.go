package settings

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// generalPane is what the General pane holds: maild's preferences are in
// the window's data.
type generalPane struct {
	names   [7]string // what the flag fields hold
	synced  []string  // the stored names they last took
	focused int       // the flag field with the focus, -1 for none
	err     string
}

// scope is a notification scope as the drop-down offers it.
type scope struct {
	kind  api.NotifyScope
	smart int64
}

func (w *Window) scopeOptions() []option[scope] {
	options := []option[scope]{
		{scope{kind: api.NotifyScopeInbox}, "Inbox Only"},
		{scope{kind: api.NotifyScopeVips}, "VIPs"},
		{scope{kind: api.NotifyScopeContacts}, "Contacts"},
		{scope{kind: api.NotifyScopeAll}, "All Mailboxes"},
	}
	seen := map[string]int{}
	for _, s := range w.data.smart {
		label := "Smart Mailbox: " + s.Name
		seen[label]++
		if n := seen[label]; n > 1 {
			label = fmt.Sprintf("%s (%d)", label, n)
		}
		options = append(options, option[scope]{scope{kind: api.NotifyScopeSmart, smart: s.ID}, label})
	}
	return options
}

var undoOptions = []option[int64]{{0, "Off"}, {10, "10 Seconds"}, {20, "20 Seconds"}, {30, "30 Seconds"}}

var blockOptions = []option[api.BlockAction]{
	{api.BlockActionMark, "Mark it, and leave it where it arrives"},
	{api.BlockActionTrash, "Move it to the Trash"},
}

// generalGroups are the General pane's rows.
func (w *Window) generalGroups() []frostui.SettingsGroup {
	g := w.gen
	s := w.data.settings
	off := s == nil
	w.syncFlagNames(g)
	current := scope{kind: api.NotifyScopeInbox}
	delay := int64(10)
	block := api.BlockActionMark
	if s != nil {
		current, delay = scope{kind: s.NotifyScope}, s.UndoDelay
		if s.NotifyScope == api.NotifyScopeSmart && s.NotifySmartID != nil {
			current.smart = *s.NotifySmartID
		}
		if s.BlockAction != nil {
			block = *s.BlockAction
		}
	}
	notify := frostui.Setting{Title: "New message notifications", Control: func(c *ui.Context) {
		choose(c, "New message notifications", current, w.scopeOptions(), off, func(next scope) {
			p := &api.SettingsSetParams{NotifyScope: &next.kind}
			if next.kind == api.NotifyScopeSmart {
				p.NotifySmartID = &next.smart
			}
			w.setSettings(p)
		}).Width(240).Shrink(0)
	}}
	undo := frostui.Setting{Title: "Undo send delay", Control: func(c *ui.Context) {
		choose(c, "Undo send delay", delay, undoOptions, off, func(next int64) {
			w.setSettings(&api.SettingsSetParams{UndoDelay: &next})
		}).Width(240).Shrink(0)
	}}
	blocked := frostui.Setting{Title: "Blocked mail", Control: func(c *ui.Context) {
		choose(c, "Blocked mail", block, blockOptions, off, func(next api.BlockAction) {
			w.setSettings(&api.SettingsSetParams{BlockAction: &next})
		}).Width(260).Shrink(0)
	}}
	flags := make([]frostui.Setting, len(g.names))
	for i := range g.names {
		flags[i] = frostui.Setting{Title: mail.FlagNames[i], Control: func(c *ui.Context) { w.flagField(c, g, i, off) }}
	}
	return []frostui.SettingsGroup{
		{Settings: []frostui.Setting{notify, undo, blocked}},
		{Title: "Flag Names", Settings: flags},
		{Title: "Blocked Senders", Settings: w.blockedRows()},
	}
}

// flagColor is a flag's color, light and dark (docs/specs/ui.md, Tokens:
// Flags).
func flagColor(t *ui.Theme, i int) ui.Color {
	colors := [7][2]string{{"#ff3b30", "#ff453a"}, {"#ff9500", "#ff9f0a"}, {"#ffcc00", "#ffd60a"}, {"#34c759", "#30d158"},
		{"#007aff", "#0a84ff"}, {"#af52de", "#bf5af2"}, {"#8e8e93", "#98989d"}}
	shade := 0
	if t.Dark {
		shade = 1
	}
	return ui.Hex(colors[i][shade])
}

// flagField is one flag's name, saved on Enter and as the field is left.
func (w *Window) flagField(c *ui.Context, g *generalPane, i int, off bool) {
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Width(240).Shrink(0).Children(func() {
		frostui.Icon(c, fmt.Sprintf("%s%d", icons.FlagPrefix, i+1)).FontSize(12).TextColor(flagColor(c.Theme(), i))
		in := ui.TextInput(c, &g.names[i]).Label(fmt.Sprintf("Flag %d name", i+1)).Placeholder(mail.FlagNames[i]).
			MaxLength(40).Disabled(off).Grow(1).MinWidth(0)
		if in.Submitted() {
			w.saveFlagNames(g)
		}
		switch {
		case in.Focused():
			g.focused = i
		case g.focused == i:
			g.focused = -1
			w.saveFlagNames(g)
		}
	})
}

// syncFlagNames shows the stored names whenever they change.
func (w *Window) syncFlagNames(g *generalPane) {
	if w.data.settings == nil || slices.Equal(g.synced, w.data.settings.FlagNames) && g.synced != nil {
		return
	}
	g.synced = w.data.settings.FlagNames
	for i := range g.names {
		g.names[i] = ""
		if i < len(g.synced) {
			g.names[i] = g.synced[i]
		}
	}
}

func (w *Window) saveFlagNames(g *generalPane) {
	if w.data.settings == nil {
		return
	}
	trimmed := make([]string, len(g.names))
	changed := false
	for i, name := range g.names {
		trimmed[i] = strings.TrimSpace(name)
		stored := ""
		if i < len(w.data.settings.FlagNames) {
			stored = w.data.settings.FlagNames[i]
		}
		changed = changed || trimmed[i] != stored
	}
	if changed {
		w.setSettings(&api.SettingsSetParams{FlagNames: trimmed})
	}
}

// blockedRows lists the blocked senders, by address.
func (w *Window) blockedRows() []frostui.Setting {
	if len(w.data.blocked) == 0 {
		return []frostui.Setting{{Title: "None"}}
	}
	rows := make([]frostui.Setting, 0, len(w.data.blocked))
	for _, b := range w.data.blocked {
		title, subtitle := b.Name, b.Address
		if title == "" {
			title, subtitle = b.Address, ""
		}
		rows = append(rows, frostui.Setting{Title: title, Subtitle: subtitle, Control: func(c *ui.Context) {
			if ui.Button(c, "Unblock").Label("Unblock " + b.Address).Clicked() {
				w.unblock(b.Address)
			}
		}})
	}
	return rows
}

// generalBody is the error under the rows, until the next change succeeds.
func (w *Window) generalBody(c *ui.Context) {
	if w.gen != nil {
		alert(c, w.gen.err)
	}
}

func (w *Window) setSettings(p *api.SettingsSetParams) {
	w.generalCall(func(ctx context.Context, c *api.Client) error {
		_, err := c.Settings().Set(ctx, p)
		return err
	})
}

func (w *Window) unblock(address string) {
	w.generalCall(func(ctx context.Context, c *api.Client) error {
		return c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: []string{address}})
	})
}

// generalCall runs a change; its error stays until the next change
// succeeds. Changes are not held back by one another, as each stands alone.
func (w *Window) generalCall(f func(ctx context.Context, c *api.Client) error) {
	g, cn := w.gen, w.cn
	if g == nil || cn == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(cn.ctx, callTimeout)
		defer cancel()
		err := f(ctx, cn.client)
		w.o.Post(func() {
			g.err = ""
			if err != nil {
				g.err = message(err)
			}
		})
	}()
}
