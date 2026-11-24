package main

import (
	"context"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/nsgo-dev/nsgo/core"
)

func (w *workspace) createMailbox(account int64, parent *int64) {
	w.form("New mailbox", []editField{{"Mailbox name", "", false}}, func(v []string) error {
		w.change(func(ctx context.Context, c *api.Client) error {
			_, err := c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: account, ParentID: parent, Name: v[0]})
			return err
		}, w.closePanel)
		return nil
	})
}
func (w *workspace) mailboxMenu(a api.Account, b api.Mailbox) {
	w.show(func(cx *core.Scope) core.Node {
		st := w.store.State()
		var role core.Node
		nodes := []core.Node{action("Open mailbox", func() { w.closePanel(); w.navigate(mail.Source{Kind: mail.Mailbox, ID: b.ID}.Key()) })}
		if s := st.Settings; s != nil {
			index := slices.Index(s.Favorites, b.ID)
			add := index < 0
			for _, step := range []struct {
				label string
				delta int
			}{{"Move favorite up", -1}, {"Move favorite down", 1}} {
				pos := index + step.delta
				if index < 0 || pos < 0 || pos >= len(s.Favorites) {
					continue
				}
				nodes = append(nodes, action(step.label, func() {
					favorites := slices.Clone(s.Favorites)
					favorites[index], favorites[pos] = favorites[pos], favorites[index]
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Settings().Set(ctx, &api.SettingsSetParams{Favorites: favorites})
						return err
					}, w.closePanel)
				}))
			}
			label := "Remove from favorites"
			if add {
				label = "Add to favorites"
			}
			nodes = append(nodes, action(label, func() {
				next := slices.Clone(s.Favorites)
				if add {
					next = append(next, b.ID)
				} else {
					next = slices.DeleteFunc(next, func(v int64) bool { return v == b.ID })
				}
				w.change(func(ctx context.Context, c *api.Client) error {
					_, err := c.Settings().Set(ctx, &api.SettingsSetParams{Favorites: next})
					return err
				}, w.closePanel)
			}))
		}
		if !a.ReadOnly {
			nodes = append(nodes, action("Mark all as read", func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					_, err := c.Mailbox().MarkRead(ctx, &api.MailboxMarkReadParams{ID: b.ID})
					return err
				}, w.closePanel)
			}), action("New submailbox", func() { w.createMailbox(a.ID, &b.ID) }))
			if b.Role == api.MailboxRoleNone {
				nodes = append(nodes, action("Rename mailbox", func() {
					w.form("Rename mailbox", []editField{{"Mailbox name", b.Name, false}}, func(v []string) error {
						w.change(func(ctx context.Context, c *api.Client) error {
							_, err := c.Mailbox().Rename(ctx, &api.MailboxRenameParams{ID: b.ID, Name: v[0]})
							return err
						}, w.closePanel)
						return nil
					})
				}), action("Move mailbox", func() {
					w.show(func(*core.Scope) core.Node {
						move := func(parent *int64) {
							w.change(func(ctx context.Context, c *api.Client) error {
								_, err := c.Mailbox().Move(ctx, &api.MailboxMoveParams{ID: b.ID, ParentID: parent})
								return err
							}, w.closePanel)
						}
						choices := []core.Node{action("Account root", func() { move(nil) })}
						for _, p := range st.Mailboxes {
							if p.AccountID == a.ID && p.ID != b.ID {
								choices = append(choices, action(p.Path, func() { move(&p.ID) }))
							}
						}
						return w.dialog("Choose parent mailbox", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(400)}, Children: []core.Node{actionList(choices...)}}})
					})
				}), destroying("Delete mailbox", func() {
					w.destroy("Delete mailbox?", b.Path+" and its child mailboxes will be removed.", "Delete", func() {
						w.change(func(ctx context.Context, c *api.Client) error {
							return c.Mailbox().Delete(ctx, &api.MailboxDeleteParams{ID: b.ID})
						}, nil)
					})
				}))
			}
			if b.Role == api.MailboxRoleTrash || b.Role == api.MailboxRoleJunk {
				nodes = append(nodes, destroying("Empty mailbox", func() {
					w.destroy("Permanently empty mailbox?", b.Path, "Erase all messages", func() {
						w.change(func(ctx context.Context, c *api.Client) error {
							_, err := c.Mailbox().Erase(ctx, &api.MailboxEraseParams{ID: b.ID})
							return err
						}, nil)
					})
				}))
			}
			if a.Kind != api.AccountKindGmail {
				role = choice("Use mailbox for", b.Role, named(api.MailboxRoleDrafts, api.MailboxRoleSent, api.MailboxRoleJunk, api.MailboxRoleTrash, api.MailboxRoleArchive), func(v api.MailboxRole) {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Mailbox().SetRole(ctx, &api.MailboxSetRoleParams{ID: b.ID, Role: v})
						return err
					}, w.closePanel)
				}, cx)
			}
		}
		panel := []core.Node{actionList(nodes...)}
		if role != nil {
			panel = append(panel, role)
		}
		return w.dialog(b.Path, panel)
	})
}
func (w *workspace) saveSearch() {
	p := api.SmartFromSearchParams{Text: w.search}
	if !w.listState.SearchAll && w.source.Kind == mail.Mailbox {
		id := w.source.ID
		p.MailboxID = &id
	}
	perform(w, func(ctx context.Context, c *api.Client) (*api.Conditions, error) {
		return c.Smart().FromSearch(ctx, &p)
	}, func(c *api.Conditions, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.filterForm(api.Rule{}, &api.SmartMailbox{Name: p.Text, Conditions: *c, IncludeSent: true, IncludeTrash: true})
	})
}
