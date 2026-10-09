package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/store"
)

// showcaseList is a made-up CalDAV task list.
type showcaseList struct {
	name  string
	tasks []showcaseTask
}

// showcaseTask is a task due days after today (nil: no date), done when
// done; sub tasks follow their parent.
type showcaseTask struct {
	title, notes string
	due          *int
	done         bool
	sub          []showcaseTask
}

func in(days int) *int { return &days }

// showcaseTaskLists are the showcase accounts' task lists: a working week's
// to-dos with an overdue one, a subtask and one done, and a personal list.
func showcaseTaskLists() map[string][]showcaseList {
	return map[string][]showcaseList{
		"ann@northwind.example": {
			{name: "Work", tasks: []showcaseTask{
				{title: "Send the launch checklist to Maria", notes: "After the design review\nAttach the risk list", due: in(0)},
				{title: "Review Omar's pull request", due: in(-1)},
				{title: "Book the Lisbon venue", due: in(2), sub: []showcaseTask{
					{title: "Compare three quotes"}, {title: "Ask finance for the PO", done: true},
				}},
				{title: "Draft the Q4 OKRs", notes: "Two pages at most", due: in(5)},
				{title: "Update the on-call runbook"},
				{title: "Ship the beta", due: in(-2), done: true},
			}},
		},
		"ann.lee@example.com": {
			{name: "Personal", tasks: []showcaseTask{
				{title: "Pick up the dry cleaning", due: in(0)},
				{title: "Renew the passport", due: in(20), notes: "Photos first"},
				{title: "Call the plumber"},
			}},
		},
	}
}

// addShowcaseTasks turns tasks on for an account and stores its task lists'
// VTODOs, indexed as a pass of pimsync would.
func addShowcaseTasks(ctx context.Context, db *store.DB, accountID int64, email string, now time.Time) error {
	lists := showcaseTaskLists()[email]
	if len(lists) == 0 {
		return nil
	}
	y, m, d := now.In(time.Local).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	domain := email[strings.LastIndexByte(email, '@')+1:]
	return db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, accountID, api.ServiceKindTasks, true, "https://dav."+domain+"/"); err != nil {
			return err
		}
		if err := tx.ServiceSynced(ctx, accountID, api.ServiceKindTasks, now.Add(-4*time.Minute), ""); err != nil {
			return err
		}
		remote := make([]store.RemoteCollection, len(lists))
		for i, l := range lists {
			remote[i] = store.RemoteCollection{Href: "/tasks/" + strings.ToLower(l.name) + "/", Name: l.name, Components: []string{"VTODO"}}
		}
		cols, err := tx.ReplaceCollections(ctx, accountID, api.CollectionKindTasklist, remote)
		if err != nil {
			return err
		}
		n := 0
		var put func(col store.Collection, t showcaseTask, parent string) error
		put = func(col store.Collection, t showcaseTask, parent string) error {
			n++
			uid := fmt.Sprintf("task-%d@showcase", n)
			c := calendar.TodoChange{Summary: &t.title, Description: &t.notes, Completed: &t.done}
			if t.due != nil {
				due := today.AddDate(0, 0, *t.due).Format(time.DateOnly)
				c.Due = &due
			}
			raw := calendar.NewTodo(uid, c, parent, now.Add(-time.Duration(100-n)*time.Hour))
			todo, err := calendar.ParseTodo(raw, calendar.Options{Local: time.Local})
			if err != nil {
				return err
			}
			id, err := tx.PutObject(ctx, store.Object{CollectionID: col.ID, Href: col.Href + uid + ".ics", ETag: `"1"`,
				Kind: store.ObjectVTodo, UID: uid, Raw: raw})
			if err != nil {
				return err
			}
			row := store.TaskRow{UID: uid, ParentUID: parent, Title: todo.Summary, Notes: todo.Description, Due: todo.Due,
				Completed: todo.Completed, Position: fmt.Sprintf("%05d", n)}
			if !todo.CompletedAt.IsZero() {
				row.CompletedAt = &todo.CompletedAt
			}
			if err := tx.IndexTask(ctx, id, row); err != nil {
				return err
			}
			for _, s := range t.sub {
				if err := put(col, s, uid); err != nil {
					return err
				}
			}
			return nil
		}
		for i, l := range lists {
			for _, t := range l.tasks {
				if err := put(cols[i], t, ""); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
