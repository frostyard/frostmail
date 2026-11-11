package mainwin_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

func TestSwitchingModuleSavesTaskEdits(t *testing.T) {
	for _, target := range []struct {
		name string
		key  ui.Key
	}{{"Mail", ui.Key1}, {"Calendar", ui.Key2}, {"People", ui.Key3}} {
		for _, field := range []string{"Title", "Notes"} {
			t.Run(target.name+"/"+field, func(t *testing.T) {
				f := newFixture(t, 1)
				var listID int64
				if err := f.srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
					if err := tx.SetService(t.Context(), f.acct, api.ServiceKindTasks, true, "https://dav.a.test/"); err != nil {
						return err
					}
					lists, err := tx.ReplaceCollections(t.Context(), f.acct, api.CollectionKindTasklist, []store.RemoteCollection{{Href: "/tasks/", Name: "Tasks"}})
					if err == nil {
						listID = lists[0].ID
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				client := f.srv.Dial(t)
				notes := "Original notes"
				task, err := client.Tasks().Create(t.Context(), &api.TasksCreateParams{ListID: &listID, Title: "Report", Notes: &notes})
				if err != nil {
					t.Fatal(err)
				}
				f.until(t, "Mail", func() bool { return f.tt.HasText("Message 1") })
				f.tt.Key(ui.Cmd, ui.Key4)
				f.until(t, "Tasks", func() bool { return f.tt.HasText("All Tasks") })
				if err := f.tt.Click("All Tasks"); err != nil {
					t.Fatal(err)
				}
				f.until(t, "the task", func() bool { return f.tt.HasText(notes) })
				if err := f.tt.Click(notes); err != nil {
					t.Fatal(err)
				}
				f.until(t, "task details", func() bool { return f.tt.HasText("Delete Task") })
				if field == "Title" {
					if err := f.tt.Click("Title"); err != nil {
						t.Fatal(err)
					}
				} else {
					r, ok := f.tt.Find("Notes")
					if !ok {
						t.Fatal("no Notes label")
					}
					f.tt.ClickAt(r.X+r.W/2, r.Y+r.H+40)
				}
				f.tt.Type("zz")
				f.tt.Key(ui.Cmd, target.key)
				f.until(t, "the module change", func() bool { return !f.tt.HasText("Delete Task") })
				f.until(t, "the saved edit", func() bool {
					got, err := client.Tasks().Get(t.Context(), &api.TasksGetParams{ID: task.ID})
					if err != nil {
						return false
					}
					if field == "Title" {
						return strings.Contains(got.Title, "zz")
					}
					return strings.Contains(got.Notes, "zz")
				})
				f.tt.Key(ui.Cmd, ui.Key4)
				f.until(t, "the task on return", func() bool { return f.tt.HasText("Delete Task") })
				if !slices.ContainsFunc(f.tt.AccessAll(), func(info ui.AccessInfo) bool { return info.Name == field && strings.Contains(info.Value, "zz") }) {
					t.Errorf("the returned %s field lost the edit", field)
				}
			})
		}
	}
}
