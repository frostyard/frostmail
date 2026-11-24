package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// A collection refreshes after a relevant maild event or reconnect. Only
// the latest request may replace the list; changing modules disposes it.
func collection[T any](cx *core.Scope, w *workspace, match func(api.Event) bool, fetch func() func(context.Context, *api.Client) ([]T, error)) (*core.Signal[[]T], func()) {
	rows := core.NewSignalFunc(cx, []T(nil), func(_, _ []T) bool { return false })
	revision := 0
	load := func() {
		revision++
		ticket := revision
		if w.session.Client() == nil {
			return
		}
		perform(w, fetch(), func(v []T, err error) {
			cx.Post(func() {
				if ticket != revision {
					return
				}
				if err != nil {
					w.problem.Set(err.Error())
					return
				}
				rows.Set(v)
			})
		})
	}
	// Coalesce notification bursts before issuing another list query.
	var scheduled func() bool
	cx.Cleanup(w.events.On(func(ev api.Event) {
		if match(ev) {
			cx.Post(func() {
				if scheduled != nil {
					return
				}
				scheduled = cx.AfterFunc(100*time.Millisecond, func() { scheduled = nil; load() })
			})
		}
	}))
	cx.Effect(func() { w.status.Get(); load() })
	return rows, load
}
func collectionList[T any](rows *core.Signal[[]T], label string, title, subtitle func(T) string, show func(T)) core.Node {
	return ui.List{Label: label, Style: ui.FillRest, EstimatedSize: 62, Count: func() int { return len(rows.Get()) }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node {
			all := rows.Get()
			i := row.Index()
			if i < 0 || i >= len(all) {
				return nil
			}
			v := all[i]
			return workspaceui.Row(ui.Button{Text: title(v), Variant: ui.ButtonGhost, OnPress: func() { show(v) }}, core.Text{Content: subtitle(v), MaxLines: 1, Style: ui.TypeCaption})
		})
	}, Empty: core.Text{Content: "No matching items"}}
}

type editField struct {
	label, value string
	multi        bool
}

func (w *workspace) form(title string, fields []editField, submit func([]string) error) {
	w.show(func(cx *core.Scope) core.Node {
		values := make([]string, len(fields))
		children := make([]core.Node, 0, len(fields)+1)
		problem := core.NewSignal(cx, "")
		for i, f := range fields {
			values[i] = f.value
			children = append(children, workspaceui.Stack(core.Text{Content: f.label, Style: ui.TypeCaption}, ui.Input{Label: f.label, DefaultValue: f.value, Multiline: f.multi, OnChange: func(v string) { values[i] = v }}))
		}
		children = append(children, core.Text{ContentFunc: problem.Get})
		return w.dialog(title, []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(480)}, Children: children}}, ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: w.closePanel}, ui.Button{Text: "Save", Disabled: w.acting.Get, OnPress: func() {
			if err := submit(slices.Clone(values)); err != nil {
				problem.Set(err.Error())
			}
		}})
	})
}
func (w *workspace) tasks(cx *core.Scope) core.Node {
	list := core.NewSignal(cx, int64(0))
	completed := core.NewSignal(cx, false)
	rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.TasksChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Task, error) {
		id := optionalID(list.GetUntracked())
		value := completed.GetUntracked()
		return func(ctx context.Context, c *api.Client) ([]api.Task, error) {
			return c.Tasks().List(ctx, &api.TasksListParams{ListID: id, Completed: &value})
		}
	})
	mode := core.NewSignal(cx, "All tasks")
	watchFlagged := func(owner *core.Scope) core.Node {
		flagged := w.flaggedMessages(owner, 200)
		return core.Scroll{Style: ui.FillRest, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			var items []core.Node
			for _, m := range flagged() {
				items = append(items, w.flagTaskRow(m))
			}
			if len(items) == 0 {
				return core.Text{Content: "No flagged mail"}
			}
			return workspaceui.Stack(items...)
		})}}
	}
	return core.View{Style: ui.FillRest, Children: []core.Node{
		ui.Tabs[string]{Label: "Task view", Options: named("Today", "All tasks", "Flagged mail"), Value: mode.Get, OnChange: mode.Set},
		core.Dynamic(func(owner *core.Scope) core.Node {
			tab := mode.Get()
			if tab == "Flagged mail" {
				return watchFlagged(owner)
			}
			today := time.Now().Format(time.DateOnly)
			newTask := func() {
				t := api.Task{ListID: list.GetUntracked()}
				if tab == "Today" {
					t.Due = today
				}
				w.taskForm(t, load)
			}
			w.moduleKey(owner, "mod+n", newTask)
			shown := core.NewMemoFunc(owner, func() []api.Task {
				all := rows.Get()
				if tab != "Today" {
					return all
				}
				var due []api.Task
				for _, t := range all {
					if !t.Completed && t.Due != "" && t.Due <= today {
						due = append(due, t)
					}
				}
				return due
			}, slices.Equal)
			items := ui.List{Label: "Tasks", Style: ui.FillRest, EstimatedSize: 52, Count: func() int { return len(shown.Get()) }, Row: func(_ *core.Scope, row core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					all := shown.Get()
					i := row.Index()
					if i < 0 || i >= len(all) {
						return nil
					}
					t := all[i]
					inset := core.Points(0)
					if t.ParentID != nil && slices.ContainsFunc(all[:i], func(other api.Task) bool { return other.ID == *t.ParentID }) {
						inset = 24
					}
					label := t.Due
					if t.MessageID != nil {
						label = strings.TrimSpace(label + "  · From mail")
					}
					return core.View{Style: core.Style{Padding: core.Edges{Left: inset}}, Children: []core.Node{workspaceui.Row(ui.Checkbox{Label: "Completed: " + t.Title, Checked: func() bool { return t.Completed }, Disabled: func() bool { return t.ReadOnly }, OnChange: func(done bool) { w.setTaskCompleted(t, done, load) }}, button(t.Title, func() { w.taskDetail(t, load) }), core.Text{Content: label, Style: ui.TypeCaption})}}
				})
			}, Empty: core.Text{Content: "No tasks"}}
			bar := []core.Node{ui.Button{Text: "New task", OnPress: newTask}}
			if tab != "Today" {
				bar = append(bar, ui.Checkbox{Text: "Include completed", Checked: completed.Get, OnChange: func(v bool) { completed.Set(v); load() }})
			}
			return core.View{Style: ui.FillRest, Children: []core.Node{workspaceui.Row(bar...), w.collectionFilter(owner, "Task lists", func(c api.Collection) bool { return c.Tasks }, list, load), items}}
		}),
	}}
}
func (w *workspace) taskDetail(t api.Task, load func()) {
	w.show(func(*core.Scope) core.Node {
		children := []core.Node{core.Text{Content: t.Notes}, core.Text{Content: t.Due, Style: ui.TypeCaption}}
		if t.MessageID != nil {
			children = append(children, button("Open linked message", func() { w.closePanel(); w.module.Set("Mail"); w.openMessage(*t.MessageID, false) }))
		}
		if !t.ReadOnly {
			children = append(children, button("New subtask", func() { w.taskForm(api.Task{ListID: t.ListID, ParentID: &t.ID}, load) }))
			done := !t.Completed
			label := "Complete"
			if !done {
				label = "Reopen"
			}
			children = append(children, workspaceui.Row(ui.Button{Text: label, OnPress: func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					_, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: t.ID, Completed: &done})
					return err
				}, func() { w.closePanel(); load() })
			}}, ui.Button{Text: "Edit task", OnPress: func() { w.taskForm(t, load) }}, ui.Button{Text: "Delete task", Variant: ui.ButtonDanger, OnPress: func() {
				w.confirm("Delete task?", t.Title, "Delete", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						return c.Tasks().Delete(ctx, &api.TasksDeleteParams{ID: t.ID})
					}, load)
				})
			}}))
		}
		return w.dialog(t.Title, children)
	})
}
func changed(old, v string) *string {
	if old == v {
		return nil
	}
	return &v
}
func (w *workspace) taskForm(t api.Task, load func()) {
	w.form("Task", []editField{{"Title", t.Title, false}, {"Notes", t.Notes, true}, {"Due date (YYYY-MM-DD)", t.Due, false}}, func(v []string) error {
		if strings.TrimSpace(v[0]) == "" {
			return fmt.Errorf("Enter a task title")
		}
		if v[2] != "" {
			if _, err := time.Parse(time.DateOnly, v[2]); err != nil {
				return err
			}
		}
		w.change(func(ctx context.Context, c *api.Client) error {
			if t.ID == 0 {
				_, err := c.Tasks().Create(ctx, &api.TasksCreateParams{ListID: optionalID(t.ListID), ParentID: t.ParentID, Title: v[0], Notes: &v[1], Due: changed("", v[2])})
				return err
			}
			_, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: t.ID, Title: changed(t.Title, v[0]), Notes: changed(t.Notes, v[1]), Due: changed(t.Due, v[2])})
			return err
		}, func() { w.closePanel(); load() })
		return nil
	})
}
func (w *workspace) people(cx *core.Scope) core.Node {
	book := core.NewSignal(cx, int64(0))
	var query string
	rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.PeopleChanged); return ok }, func() func(context.Context, *api.Client) ([]api.PersonSummary, error) {
		id := optionalID(book.GetUntracked())
		q := query
		return func(ctx context.Context, c *api.Client) ([]api.PersonSummary, error) {
			return c.People().List(ctx, &api.PeopleListParams{CollectionID: id, Query: changed("", q)})
		}
	})
	newPerson := func() { w.contactForm(api.Contact{CollectionID: book.GetUntracked()}, load) }
	w.moduleKey(cx, "mod+n", newPerson)
	return core.View{Style: ui.FillRest, Children: []core.Node{workspaceui.Row(ui.Input{Label: "Find a person", Placeholder: "Name or address", OnSubmit: func(v string) { query = v; load() }}, ui.Button{Text: "New contact", OnPress: newPerson}), w.collectionFilter(cx, "Address books", func(c api.Collection) bool { return c.Kind == api.CollectionKindAddressbook }, book, load), collectionList(rows, "Contacts", func(p api.PersonSummary) string { return p.DisplayName }, func(p api.PersonSummary) string { return p.Email }, func(p api.PersonSummary) {
		perform(w, func(ctx context.Context, c *api.Client) (*api.Person, error) {
			return c.People().Get(ctx, &api.PeopleGetParams{ID: p.ID})
		}, func(person *api.Person, err error) {
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			w.personDetail(person, load)
		})
	})}}
}
func (w *workspace) personDetail(p *api.Person, load func()) {
	w.show(func(cx *core.Scope) core.Node {
		var cards []core.Node
		if p.HasPhoto {
			cards = append(cards, w.contactPhoto(cx, p.ID))
		}
		for _, c := range p.Contacts {
			children := []core.Node{core.Text{Content: c.DisplayName, Style: ui.TypeTitle}, core.Text{Content: c.Organization}, core.Text{Content: c.Note}}
			for _, address := range c.Emails {
				children = append(children, ui.Button{Text: address.Value, Variant: ui.ButtonGhost, OnPress: func() {
					perform(w, func(ctx context.Context, client *api.Client) (*api.Draft, error) {
						return client.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, To: []api.Address{{Name: p.DisplayName, Address: address.Value}}})
					}, w.editDraft)
				}})
			}
			for _, n := range c.Phones {
				children = append(children, core.Text{Content: n.Label + " " + n.Value})
			}
			children = append(children, core.Text{Content: strings.Join([]string{c.Title, c.Nickname, c.Birthday}, " ")})
			for _, u := range c.URLs {
				children = append(children, core.Text{Content: u.Label + " " + u.Value, Selectable: core.SelectableOn})
			}
			for _, a := range c.Addresses {
				children = append(children, core.Text{Content: strings.Join([]string{a.Street, a.Locality, a.Region, a.Postcode, a.Country}, " "), Selectable: core.SelectableOn})
			}
			if !c.ReadOnly {
				children = append(children, button("Edit contact details", func() { w.fullContact(c, load) }), button("Choose contact photo", func() { w.pickContactPhoto(c.ID, load) }))
				if p.HasPhoto {
					children = append(children, button("Remove contact photo", func() { w.setContactPhoto(c.ID, nil, load) }))
				}
				children = append(children, workspaceui.Row(ui.Button{Text: "Edit contact", OnPress: func() { w.contactForm(c, load) }}, ui.Button{Text: "Delete contact", Variant: ui.ButtonGhost, OnPress: func() {
					w.confirm("Delete contact?", c.DisplayName, "Delete", func() {
						w.writeContact(func(ctx context.Context, client *api.Client) (*api.ContactWrite, error) {
							return client.People().Delete(ctx, &api.PeopleDeleteParams{ID: c.ID})
						}, load)
					})
				}}))
			}
			cards = append(cards, workspaceui.Card{Children: children})
		}
		return w.dialog(p.DisplayName, []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(480)}, Children: cards}})
	})
}
func (w *workspace) contactForm(c api.Contact, load func()) {
	email := ""
	if len(c.Emails) > 0 {
		email = c.Emails[0].Value
	}
	w.form("Contact", []editField{{"Given name", c.GivenName, false}, {"Family name", c.FamilyName, false}, {"Organization", c.Organization, false}, {"Email", email, false}, {"Notes", c.Note, true}}, func(v []string) error {
		if strings.TrimSpace(v[0]+v[1]+v[2]+v[3]) == "" {
			return fmt.Errorf("Enter a name, organization or address")
		}
		f := api.ContactFields{GivenName: changed(c.GivenName, v[0]), FamilyName: changed(c.FamilyName, v[1]), Organization: changed(c.Organization, v[2]), Note: changed(c.Note, v[4])}
		if email != v[3] {
			f.Emails = append([]api.LabeledValue{}, c.Emails...)
			if len(f.Emails) > 0 {
				if v[3] == "" {
					f.Emails = f.Emails[1:]
				} else {
					f.Emails[0].Value = v[3]
				}
			} else {
				f.Emails = append(f.Emails, api.LabeledValue{Value: v[3]})
			}
		}
		w.writeContact(func(ctx context.Context, client *api.Client) (*api.ContactWrite, error) {
			if c.ID == 0 {
				return client.People().Create(ctx, &api.PeopleCreateParams{CollectionID: optionalID(c.CollectionID), Fields: f})
			}
			return client.People().Update(ctx, &api.PeopleUpdateParams{ID: c.ID, Fields: f})
		}, load)
		return nil
	})
}

func optionalID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

func (w *workspace) collectionFilter(cx *core.Scope, label string, include func(api.Collection) bool, chosen *core.Signal[int64], load func()) core.Node {
	collections, _ := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.AccountChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Collection, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Collection, error) {
			return c.Account().Collections(ctx, &api.AccountCollectionsParams{})
		}
	})
	return ui.Select[int64]{Label: label, OptionsFunc: func() []ui.Option[int64] {
		opts := []ui.Option[int64]{{Value: 0, Label: "All · " + label}}
		for _, c := range collections.Get() {
			if include(c) && c.Enabled {
				opts = append(opts, ui.Option[int64]{Value: c.ID, Label: c.Name})
			}
		}
		return opts
	}, Value: chosen.Get, OnChange: func(v int64) { chosen.Set(v); load() }}
}

func (w *workspace) reminders() {
	w.show(func(cx *core.Scope) core.Node {
		rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.CalendarReminders); return ok }, func() func(context.Context, *api.Client) ([]api.Reminder, error) {
			return func(ctx context.Context, c *api.Client) ([]api.Reminder, error) {
				return c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
			}
		})
		nodes := core.Dynamic(func(*core.Scope) core.Node {
			var cards []core.Node
			for _, r := range rows.Get() {
				ids := []string{r.ID}
				cards = append(cards, workspaceui.Card{Children: []core.Node{core.Text{Content: r.Summary, Style: ui.TypeTitle}, core.Text{Content: r.Location}, workspaceui.Row(button("Snooze 15 minutes", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						return c.Calendar().Snooze(ctx, &api.CalendarSnoozeParams{IDs: ids, Until: time.Now().Add(15 * time.Minute)})
					}, load)
				}), button("Dismiss", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						return c.Calendar().Dismiss(ctx, &api.CalendarDismissParams{IDs: ids})
					}, load)
				}))}})
			}
			if len(cards) == 0 {
				return core.Text{Content: "No reminders due"}
			}
			return workspaceui.Stack(cards...)
		})
		return w.dialog("Calendar reminders", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(440)}, Children: []core.Node{nodes}}})
	})
	w.routePanel("/reminders")
}
