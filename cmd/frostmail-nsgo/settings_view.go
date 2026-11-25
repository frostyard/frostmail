package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Settings (plan 0016, Phase 4b) is NSGo's settings view: categories at the
// side, groups of rows, and a search through every row. A row is NSGo's
// own, so an account, a rule or a snippet is found by its name as a
// preference is. What a row does (its forms, its confirmations) is where it
// was: accounts.go, settings.go, organize.go and mailboxes.go.

// action and destroying are rows of a list of actions, which a panel shows
// in place of a stack of buttons: a row is pressed, and has a chevron.
func action(label string, press func()) core.Node {
	return ui.FormRow{Title: label, OnPress: press}
}

func destroying(label string, press func()) core.Node {
	return ui.FormRow{Title: label, Variant: ui.FormRowDanger, OnPress: press}
}

// actionList is a panel's actions as one section.
func actionList(rows ...core.Node) core.Node { return ui.FormSection{Children: rows} }

// small is a button at a row's end.
func small(b ui.Button) ui.Button { b.Size = ui.SizeSmall; return b }

// rowButton is an icon button at a row's end, named for what it acts on.
func rowButton(label string, icon func() *core.SVG, press func()) core.Node {
	return ui.IconButton{Label: label, Icon: icon, OnPress: press}
}

// settings is the Settings module.
func (w *workspace) settings(cx *core.Scope) core.Node {
	// Settings opens on Accounts, or on the category a caller asked for.
	start := "accounts"
	if w.settingsStart != "" {
		start, w.settingsStart = w.settingsStart, ""
	}
	category, query := core.NewSignal(cx, start), core.NewSignal(cx, "")
	identities, loadIdentities := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.AccountChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Identity, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Identity, error) {
			return c.Identity().List(ctx, &api.IdentityListParams{})
		}
	})
	snippets, loadSnippets := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.SnippetChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Snippet, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Snippet, error) {
			return c.Snippet().List(ctx, &api.SnippetListParams{})
		}
	})
	// The view is built again when what it lists changes, not with every
	// change of the store: a count that moves during a sync leaves an open
	// menu and the scroll where they are. Its controls read their values
	// as they draw.
	shape := core.NewMemo(cx, w.settingsShape)
	// The categories are the window's navigation while Settings is shown,
	// so the view draws the chosen one alone, or, while the search has
	// words, the rows of any of them that match. The search field stands
	// over the view and outside it: the view is built again as the words
	// change, and a field inside it would lose what is being typed.
	w.useNav(cx, moduleNav{
		sections: func() []ui.SidebarSection {
			var items []ui.SidebarItem
			for _, c := range settingsCategories {
				if c.id != "server" || desktopServer {
					items = append(items, navRow(c.id, c.title, c.icon))
				}
			}
			return []ui.SidebarSection{{Title: "Settings", Items: items}}
		},
		selected: category.Get,
		navigate: func(id string) { query.Set(""); category.Set(id) },
	})
	search := workspaceui.SearchField{Placeholder: "Search settings", Value: query.Get, OnChange: query.Set}
	bar := moduleBar(core.View{Style: core.Style{Width: core.Pt(280), FlexShrink: 1}, Children: []core.Node{search}})
	return core.View{TestID: "settings", Style: ui.FillRest.Merge(core.Style{Gap: 8}), Children: []core.Node{bar, core.Dynamic(func(cx *core.Scope) core.Node {
		shape.Get()
		st := core.Untracked(cx, func() live.MailState { return w.store.State() })
		categories := []ui.SettingsCategory{
			w.accountSettings(st),
			w.generalSettings(cx, st),
			w.identitySettings(identities.Get(), loadIdentities),
			w.snippetSettings(snippets.Get(), loadSnippets),
			w.filterSettings(st),
			w.mailboxSettings(st),
			w.signInSettings(),
		}
		if desktopServer {
			categories = append(categories, ui.SettingsCategory{ID: "server", Title: "Server", Icon: lucide.Server, Groups: []ui.SettingsGroup{{ID: "server", Title: "Mail server", Items: []ui.SettingsItem{
				{ID: "server", Title: "Mail server", Keywords: []string{"lodge", "maild", "local"}, Content: core.Dynamic(w.serverPanel)}}}}})
		}
		if words := strings.TrimSpace(query.Get()); words != "" {
			categories = matching(categories, words)
		} else {
			chosen := category.Get()
			categories = slices.DeleteFunc(categories, func(c ui.SettingsCategory) bool { return c.ID != chosen })
		}
		if len(categories) == 0 {
			return workspaceui.EmptyState{Icon: lucide.SearchX, Title: "No setting matches", Hint: "Try other words."}
		}
		return ui.Settings{Categories: categories, Category: category, HideSidebar: true, HideSearch: true, HideHeading: true}
	})}}
}

// matching are the categories' rows that NSGo's search finds for words, in
// their categories and groups, with those that hold none left out.
func matching(categories []ui.SettingsCategory, words string) []ui.SettingsCategory {
	var out []ui.SettingsCategory
	at := map[[2]int][2]int{} // a category and a group, to where they are in out
	for _, r := range ui.SearchSettings(categories, words) {
		place, ok := at[[2]int{r.Category, -1}]
		if !ok {
			c := categories[r.Category]
			c.Groups = nil
			out = append(out, c)
			place = [2]int{len(out) - 1, 0}
			at[[2]int{r.Category, -1}] = place
		}
		group, ok := at[[2]int{r.Category, r.Group}]
		if !ok {
			g := categories[r.Category].Groups[r.Group]
			g.Items = nil
			out[place[0]].Groups = append(out[place[0]].Groups, g)
			group = [2]int{place[0], len(out[place[0]].Groups) - 1}
			at[[2]int{r.Category, r.Group}] = group
		}
		out[group[0]].Groups[group[1]].Items = append(out[group[0]].Groups[group[1]].Items, categories[r.Category].Groups[r.Group].Items[r.Item])
	}
	return out
}

// settingsCategories are Settings' categories, as the navigation lists
// them. Server is a desktop's alone.
var settingsCategories = []struct {
	id, title string
	icon      func() *core.SVG
}{{"accounts", "Accounts", lucide.Users}, {"general", "General", lucide.Settings}, {"identities", "Identities", lucide.IdCard}, {"snippets", "Snippets", lucide.TextQuote},
	{"filters", "Filters", lucide.ListFilter}, {"mailboxes", "Mailboxes", lucide.Folder}, {"sign-in", "Sign-in", lucide.KeyRound}, {"server", "Server", lucide.Server}}

// settingsShape names what Settings lists: its rows' identities and texts,
// and nothing that changes while mail arrives.
func (w *workspace) settingsShape() string {
	w.state.Get()
	st := w.store.State()
	var b strings.Builder
	for _, a := range st.Accounts {
		fmt.Fprintf(&b, "a%d|%s|%s|%s|%v|%s;", a.ID, a.Email, a.DisplayName, a.Kind, a.ReadOnly, a.Auth)
	}
	for _, r := range st.Rules {
		problem := ""
		if r.Problem != nil {
			problem = *r.Problem
		}
		fmt.Fprintf(&b, "r%d|%s|%v|%s;", r.ID, r.Name, r.Enabled, problem)
	}
	for _, s := range st.Smart {
		fmt.Fprintf(&b, "s%d|%s;", s.ID, s.Name)
	}
	for _, m := range st.Mailboxes {
		fmt.Fprintf(&b, "m%d|%s|%s;", m.ID, m.Path, m.Role)
	}
	for _, blocked := range st.Blocked {
		fmt.Fprintf(&b, "b%s;", blocked.Address)
	}
	fmt.Fprintf(&b, "settings:%v", st.Settings != nil)
	return b.String()
}

func (w *workspace) accountSettings(st live.MailState) ui.SettingsCategory {
	group := ui.SettingsGroup{ID: "accounts", Title: "Mail accounts", Actions: []core.Node{small(primary("Add account", func() { w.startAccount() }))}}
	for _, a := range st.Accounts {
		group.Items = append(group.Items, ui.SettingsItem{ID: strconv.FormatInt(a.ID, 10), Title: a.DisplayName, Description: a.Email + " · " + string(a.Kind),
			Leading: ui.Avatar{Name: a.DisplayName, Size: 32}, Control: small(button("Manage "+a.Email, func() { w.accountMenu(a) }))})
	}
	if len(st.Accounts) == 0 {
		group.Items = []ui.SettingsItem{{ID: "none", Title: "No account yet", Description: "Add one to read its mail here."}}
	}
	return ui.SettingsCategory{ID: "accounts", Title: "Accounts", Icon: lucide.Users, Groups: []ui.SettingsGroup{group}}
}

func (w *workspace) generalSettings(cx *core.Scope, st live.MailState) ui.SettingsCategory {
	// The controls read the preferences as they draw.
	prefs := func() *api.Settings { w.state.Get(); return w.store.State().Settings }
	save := func(p api.SettingsSetParams) {
		w.change(func(ctx context.Context, c *api.Client) error { _, err := c.Settings().Set(ctx, &p); return err }, nil)
	}
	wide := core.Style{Width: core.Pt(200)}
	var look, messages []ui.SettingsItem
	if desktopServer {
		look = append(look, ui.SettingsItem{ID: "appearance", Title: "Appearance", Description: "Light, dark, or as the system. This device only.",
			Keywords: []string{"theme", "dark", "light"}, Control: workspaceui.AppearanceChoice{Value: w.appearance.Get, OnChange: w.saveAppearance}})
		sizes := []ui.Option[api.TextSize]{{Value: api.TextSizeSmaller, Label: "Smaller"}, {Value: api.TextSizeDefault, Label: "Default"}, {Value: api.TextSizeLarger, Label: "Larger"}, {Value: api.TextSizeLargest, Label: "Largest"}}
		look = append(look, ui.SettingsItem{ID: "text-size", Title: "Message text size", Description: "The size of a message's page.", Control: ui.Select[api.TextSize]{Label: "Message text size", Style: wide, Options: sizes,
			Value: func() api.TextSize {
				if s := prefs(); s != nil {
					return settingValue(s.TextSize, api.TextSizeDefault)
				}
				return api.TextSizeDefault
			}, OnChange: func(v api.TextSize) { save(api.SettingsSetParams{TextSize: &v}) }}})
	}
	var lines, delay []ui.Option[int64]
	for i := int64(0); i <= 5; i++ {
		lines = append(lines, ui.Option[int64]{Value: i, Label: strconv.FormatInt(i, 10)})
	}
	for _, i := range []int64{0, 10, 20, 30} {
		delay = append(delay, ui.Option[int64]{Value: i, Label: fmt.Sprintf("%d seconds", i)})
	}
	messages = append(messages,
		ui.SettingsItem{ID: "preview", Title: "List preview lines", Description: "How much of a message its row shows.", Control: ui.Select[int64]{Label: "List preview lines", Style: wide, Options: lines,
			Value: func() int64 {
				if s := prefs(); s != nil {
					return settingValue(s.PreviewLines, int64(2))
				}
				return 2
			}, OnChange: func(v int64) { save(api.SettingsSetParams{PreviewLines: &v}) }}},
		ui.SettingsItem{ID: "undo", Title: "Undo send delay", Description: "How long a sent message waits, so that it can be taken back.", Control: ui.Select[int64]{Label: "Undo send delay", Style: wide, Options: delay,
			Value: func() int64 {
				if s := prefs(); s != nil {
					return s.UndoDelay
				}
				return 0
			}, OnChange: func(v int64) { save(api.SettingsSetParams{UndoDelay: &v}) }}},
		ui.SettingsItem{ID: "flags", Title: "Flag names", Description: "The names of the 7 flag colors.", Control: small(ui.Button{Label: "Edit flag names", Text: "Edit…", Variant: ui.ButtonSecondary, OnPress: w.editFlagNames})})

	scopes := []ui.Option[string]{{Value: "inbox", Label: "Inbox"}, {Value: "vips", Label: "VIPs"}, {Value: "contacts", Label: "Contacts"}, {Value: "all", Label: "All messages"}}
	for _, box := range st.Smart {
		scopes = append(scopes, ui.Option[string]{Value: fmt.Sprintf("smart:%d", box.ID), Label: box.Name})
	}
	notify := []ui.SettingsItem{{ID: "scope", Title: "New message notifications", Description: "Which new messages are announced.", Control: ui.Select[string]{Label: "New message notifications", Style: wide, Options: scopes,
		Value: func() string {
			s := prefs()
			if s == nil {
				return ""
			}
			if s.NotifyScope == api.NotifyScopeSmart && s.NotifySmartID != nil {
				return fmt.Sprintf("smart:%d", *s.NotifySmartID)
			}
			return string(s.NotifyScope)
		}, OnChange: func(v string) {
			scope := api.NotifyScope(v)
			p := api.SettingsSetParams{NotifyScope: &scope}
			if raw, ok := strings.CutPrefix(v, "smart:"); ok {
				id, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					return
				}
				scope = api.NotifyScopeSmart
				p.NotifySmartID = &id
			}
			save(p)
		}}}}
	for _, node := range w.notificationSettings(cx) {
		notify = append(notify, ui.SettingsItem{ID: "device", Title: "This device", Description: "Whether the system lets FrostMail show notifications.", Control: node})
	}
	if tray := w.traySetting(); tray != nil {
		notify = append(notify, ui.SettingsItem{ID: "tray", Title: "Close to tray", Description: "Keep running when the last window closes.", Control: tray})
	}

	blocked := []ui.SettingsItem{{ID: "action", Title: "Blocked mail", Description: "What happens to mail from a blocked sender.", Control: ui.Select[api.BlockAction]{Label: "Blocked mail", Style: wide, Options: []ui.Option[api.BlockAction]{{Value: api.BlockActionMark, Label: "Mark as blocked"}, {Value: api.BlockActionTrash, Label: "Move to Trash"}},
		Value: func() api.BlockAction {
			if s := prefs(); s != nil {
				return settingValue(s.BlockAction, api.BlockActionMark)
			}
			return api.BlockActionMark
		}, OnChange: func(v api.BlockAction) { save(api.SettingsSetParams{BlockAction: &v}) }}}}
	for _, b := range st.Blocked {
		blocked = append(blocked, ui.SettingsItem{ID: "blocked-" + b.Address, Title: b.Address, Control: small(button("Unblock "+b.Address, func() {
			w.change(func(ctx context.Context, c *api.Client) error {
				return c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: []string{b.Address}})
			}, nil)
		}))})
	}
	var groups []ui.SettingsGroup
	if len(look) > 0 {
		groups = append(groups, ui.SettingsGroup{ID: "look", Title: "Look", Items: look})
	}
	groups = append(groups, ui.SettingsGroup{ID: "messages", Title: "Messages", Items: messages},
		ui.SettingsGroup{ID: "notifications", Title: "Notifications", Items: notify},
		ui.SettingsGroup{ID: "blocked", Title: "Blocked senders", Items: blocked})
	return ui.SettingsCategory{ID: "general", Title: "General", Icon: lucide.Settings, Groups: groups}
}

// editFlagNames asks for the names of the 7 flag colors.
func (w *workspace) editFlagNames() {
	var names []string
	if s := w.store.State().Settings; s != nil {
		names = s.FlagNames
	}
	var fields []editField
	for i, n := range []string{"Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Gray"} {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		fields = append(fields, editField{n, name, false})
	}
	w.form("Flag names", fields, func(v []string) error {
		p := api.SettingsSetParams{FlagNames: v}
		w.change(func(ctx context.Context, c *api.Client) error { _, err := c.Settings().Set(ctx, &p); return err }, w.closePanel)
		return nil
	})
}

func (w *workspace) identitySettings(rows []api.Identity, load func()) ui.SettingsCategory {
	group := ui.SettingsGroup{ID: "identities", Title: "Sender identities", Actions: []core.Node{small(primary("Add sender identity", func() { w.addIdentity(load) }))}}
	for _, i := range rows {
		controls := []core.Node{rowButton("Edit "+i.Email, lucide.Pencil, func() { w.editIdentity(i, load) })}
		if !i.IsDefault {
			controls = append(controls, rowButton("Delete "+i.Email, lucide.Trash, func() { w.deleteIdentity(i, load) }))
		}
		group.Items = append(group.Items, ui.SettingsItem{ID: strconv.FormatInt(i.ID, 10), Title: i.Email, Description: i.Name, Keywords: []string{"signature", "reply to"}, Control: workspaceui.Row(controls...)})
	}
	if len(rows) == 0 {
		group.Items = []ui.SettingsItem{{ID: "none", Title: "No sender identity", Description: "An account gets one when it is added."}}
	}
	return ui.SettingsCategory{ID: "identities", Title: "Identities", Icon: lucide.IdCard, Groups: []ui.SettingsGroup{group}}
}

func (w *workspace) snippetSettings(rows []api.Snippet, load func()) ui.SettingsCategory {
	group := ui.SettingsGroup{ID: "snippets", Title: "Message snippets", Actions: []core.Node{small(primary("Add snippet", func() { w.editSnippet(api.Snippet{}, load) }))}}
	for _, s := range rows {
		group.Items = append(group.Items, ui.SettingsItem{ID: strconv.FormatInt(s.ID, 10), Title: s.Name, Description: "Changed " + s.UpdatedAt.Local().Format("Jan 2"), Control: workspaceui.Row(
			rowButton("Edit "+s.Name, lucide.Pencil, func() { w.editSnippet(s, load) }),
			rowButton("Delete "+s.Name, lucide.Trash, func() { w.deleteSnippet(s, load) }))})
	}
	if len(rows) == 0 {
		group.Items = []ui.SettingsItem{{ID: "none", Title: "No snippet yet", Description: "A snippet is text that Compose inserts by name."}}
	}
	return ui.SettingsCategory{ID: "snippets", Title: "Snippets", Icon: lucide.TextQuote, Groups: []ui.SettingsGroup{group}}
}

func (w *workspace) filterSettings(st live.MailState) ui.SettingsCategory {
	rules := ui.SettingsGroup{ID: "rules", Title: "Rules", Actions: []core.Node{small(primary("New rule", func() { w.filterForm(api.Rule{Enabled: true}, nil) }))}}
	for i, r := range st.Rules {
		detail := "Enabled"
		if !r.Enabled {
			detail = "Disabled"
		}
		if r.Problem != nil {
			detail += " · " + *r.Problem
		}
		last := i+1 == len(st.Rules)
		rules.Items = append(rules.Items, ui.SettingsItem{ID: strconv.FormatInt(r.ID, 10), Title: r.Name, Description: detail, Control: workspaceui.Row(
			ui.IconButton{Label: "Move " + r.Name + " up", Icon: lucide.ArrowUp, Disabled: func() bool { return i == 0 }, OnPress: func() { w.moveRule(r, -1) }},
			ui.IconButton{Label: "Move " + r.Name + " down", Icon: lucide.ArrowDown, Disabled: func() bool { return last }, OnPress: func() { w.moveRule(r, 1) }},
			rowButton("Edit "+r.Name, lucide.Pencil, func() { w.filterForm(r, nil) }),
			rowButton("Duplicate "+r.Name, lucide.Copy, func() { r.ID = 0; r.Name += " Copy"; w.filterForm(r, nil) }),
			rowButton("Delete "+r.Name, lucide.Trash, func() { w.deleteRule(r) }))})
	}
	if len(st.Rules) == 0 {
		rules.Items = []ui.SettingsItem{{ID: "none", Title: "No rule yet", Description: "A rule files, flags or answers mail as it arrives."}}
	}
	smart := ui.SettingsGroup{ID: "smart", Title: "Smart mailboxes", Actions: []core.Node{small(button("New smart mailbox", func() { w.filterForm(api.Rule{}, &api.SmartMailbox{}) }))}}
	for _, s := range st.Smart {
		smart.Items = append(smart.Items, ui.SettingsItem{ID: strconv.FormatInt(s.ID, 10), Title: s.Name, Control: workspaceui.Row(
			rowButton("Edit "+s.Name, lucide.Pencil, func() { w.filterForm(api.Rule{}, &s) }),
			rowButton("Delete "+s.Name, lucide.Trash, func() { w.deleteSmart(s) }))})
	}
	if len(st.Smart) == 0 {
		smart.Items = []ui.SettingsItem{{ID: "none", Title: "No smart mailbox yet", Description: "A smart mailbox lists the messages that match its conditions."}}
	}
	return ui.SettingsCategory{ID: "filters", Title: "Filters", Icon: lucide.ListFilter, Groups: []ui.SettingsGroup{rules, smart}}
}

func (w *workspace) mailboxSettings(st live.MailState) ui.SettingsCategory {
	var groups []ui.SettingsGroup
	for _, a := range st.Accounts {
		group := ui.SettingsGroup{ID: strconv.FormatInt(a.ID, 10), Title: a.Email}
		if !a.ReadOnly {
			group.Actions = []core.Node{small(ui.Button{Label: "Create mailbox in " + a.Email, Text: "New mailbox", Variant: ui.ButtonSecondary, OnPress: func() { w.createMailbox(a.ID, nil) }})}
		}
		for _, box := range st.Mailboxes {
			if box.AccountID != a.ID {
				continue
			}
			item := ui.SettingsItem{ID: strconv.FormatInt(box.ID, 10), Title: box.Path, Leading: ui.Icon(roleIcons[box.Role], 16, func() core.Color { return ui.ColorsOf(w.cx).Accent() }),
				Control: ui.Button{Label: "Options for " + box.Path + ", " + a.Email, Icon: lucide.Ellipsis, Variant: ui.ButtonGhost, Size: ui.SizeSmall, OnPress: func() { w.mailboxMenu(a, box) }}}
			if box.Role != api.MailboxRoleNone {
				item.Description = "Used for " + string(box.Role)
			}
			group.Items = append(group.Items, item)
		}
		if len(group.Items) > 0 || len(group.Actions) > 0 {
			groups = append(groups, group)
		}
	}
	if len(groups) == 0 {
		groups = []ui.SettingsGroup{{ID: "none", Title: "Mailboxes", Items: []ui.SettingsItem{{ID: "none", Title: "No mailbox yet", Description: "An account's mailboxes are listed here."}}}}
	}
	return ui.SettingsCategory{ID: "mailboxes", Title: "Mailboxes", Icon: lucide.Folder, Groups: groups}
}

// roleIcons are a mailbox's icon by its role, as the navigation draws it.
var roleIcons = map[api.MailboxRole]func() *core.SVG{
	api.MailboxRoleNone: lucide.Folder, api.MailboxRoleInbox: lucide.Inbox, api.MailboxRoleDrafts: lucide.FilePen, api.MailboxRoleSent: lucide.Send,
	api.MailboxRoleJunk: lucide.ShieldAlert, api.MailboxRoleTrash: lucide.Trash, api.MailboxRoleArchive: lucide.Archive,
	api.MailboxRoleAll: lucide.Archive, api.MailboxRoleFlagged: lucide.Flag,
}

func (w *workspace) signInSettings() ui.SettingsCategory {
	client := func(id, name string, provider api.OAuthProvider) ui.SettingsItem {
		return ui.SettingsItem{ID: id, Title: name + " sign-in client", Description: "The client ID and secret FrostMail signs in to " + name + " with.", Keywords: []string{"oauth"},
			Control: small(ui.Button{Label: name + " sign-in client", Text: "Edit…", Variant: ui.ButtonSecondary, OnPress: func() { w.oauthClient(provider) }})}
	}
	return ui.SettingsCategory{ID: "sign-in", Title: "Sign-in", Icon: lucide.KeyRound, Groups: []ui.SettingsGroup{{ID: "clients", Title: "Sign-in clients", Items: []ui.SettingsItem{
		client("google", "Google", api.OAuthProviderGoogle), client("microsoft", "Microsoft", api.OAuthProviderMicrosoft)}}}}
}
