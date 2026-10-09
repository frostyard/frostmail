package engine_test

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// tasksServer has a Gmail account with a Google list (Work) and an IMAP
// account with a CalDAV list (Home), both with tasks on.
func tasksServer(t *testing.T) (*rpctest.Server, *api.Client, store.Collection, store.Collection) {
	t.Helper()
	srv := rpctest.Start(t)
	ctx := t.Context()
	var work, home store.Collection
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "imap.example", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
		gmail, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: "ann@gmail.example",
			Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		imap, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "ann@example.com",
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		for _, id := range []int64{gmail.ID, imap.ID} {
			if err := tx.SetService(ctx, id, api.ServiceKindTasks, true, "https://tasks.example/"); err != nil {
				return err
			}
		}
		cols, err := tx.ReplaceCollections(ctx, gmail.ID, api.CollectionKindTasklist, []store.RemoteCollection{{Href: "W", Name: "Work"}})
		if err != nil {
			return err
		}
		work = cols[0]
		if cols, err = tx.ReplaceCollections(ctx, imap.ID, api.CollectionKindTasklist, []store.RemoteCollection{{Href: "/h/", Name: "Home"}}); err != nil {
			return err
		}
		home = cols[0]
		for _, r := range []struct {
			col store.Collection
			row store.TaskRow
		}{
			{work, store.TaskRow{UID: "w1", Title: "Report", Position: "00001", Due: "2026-10-10"}},
			{work, store.TaskRow{UID: "w2", ParentUID: "w1", Title: "Charts", Position: "00001"}},
			{work, store.TaskRow{UID: "w3", Title: "Done", Position: "00002", Completed: true}},
			{home, store.TaskRow{UID: "h1", Title: "Groceries"}},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: r.col.ID, Href: r.row.UID, Kind: store.ObjectGTask, UID: r.row.UID, Raw: []byte(`{"title":"` + r.row.Title + `"}`)})
			if err != nil {
				return err
			}
			if err := tx.IndexTask(ctx, id, r.row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, srv.Dial(t), work, home
}

func taskTitles(list []api.Task) []string {
	var out []string
	for _, t := range list {
		out = append(out, t.Title)
	}
	return out
}

func TestTasksList(t *testing.T) {
	_, c, work, home := tasksServer(t)
	ctx := t.Context()
	all, err := c.Tasks().List(ctx, &api.TasksListParams{})
	if err != nil || !slices.Equal(taskTitles(all), []string{"Report", "Charts", "Groceries"}) {
		t.Fatalf("list = %q, %v", taskTitles(all), err)
	}
	report, charts := all[0], all[1]
	if report.ListID != work.ID || report.Due != "2026-10-10" || report.ParentID != nil || report.MessageID != nil || report.ReadOnly {
		t.Errorf("Report = %+v", report)
	}
	if charts.ParentID == nil || *charts.ParentID != report.ID {
		t.Errorf("Charts = %+v", charts)
	}
	done, _ := c.Tasks().List(ctx, &api.TasksListParams{ListID: &work.ID, Completed: ptr(true)})
	if !slices.Equal(taskTitles(done), []string{"Report", "Charts", "Done"}) {
		t.Errorf("Work with completed = %q", taskTitles(done))
	}
	if due, _ := c.Tasks().List(ctx, &api.TasksListParams{DueBefore: ptr("2026-10-11")}); !slices.Equal(taskTitles(due), []string{"Report"}) {
		t.Errorf("due before Oct 11 = %q", taskTitles(due))
	}
	if _, err := c.Tasks().List(ctx, &api.TasksListParams{ListID: ptr(int64(99999))}); code(err) != api.CodeNotFound {
		t.Errorf("an unknown list: %v", err)
	}
	if _, err := c.Tasks().List(ctx, &api.TasksListParams{DueBefore: ptr("soon")}); code(err) != api.CodeInvalidParams {
		t.Errorf("a bad date: %v", err)
	}
	_ = home
}

func TestTaskWritesGuarded(t *testing.T) {
	srv, c, work, home := tasksServer(t)
	ctx := t.Context()
	if _, err := c.Tasks().Create(ctx, &api.TasksCreateParams{Title: "  ", ListID: &work.ID}); code(err) != api.CodeInvalidParams {
		t.Errorf("an empty title: %v", err)
	}
	if _, err := c.Tasks().Create(ctx, &api.TasksCreateParams{Title: "x", ListID: &work.ID, Due: ptr("tomorrow")}); code(err) != api.CodeInvalidParams {
		t.Errorf("a bad due date: %v", err)
	}
	if _, err := c.Tasks().Create(ctx, &api.TasksCreateParams{Title: "x", ListID: &home.ID}); code(err) != api.CodeConflict {
		t.Errorf("a CalDAV list: %v", err)
	}
	all, _ := c.Tasks().List(ctx, &api.TasksListParams{})
	charts := all[1]
	if _, err := c.Tasks().Create(ctx, &api.TasksCreateParams{Title: "x", ParentID: &charts.ID}); code(err) != api.CodeInvalidParams {
		t.Errorf("a subtask of a subtask: %v", err)
	}
	// The default list is the first account's with tasks on.
	made, err := c.Tasks().Create(ctx, &api.TasksCreateParams{Title: "Inbox zero"})
	if err != nil || made.ListID != work.ID {
		t.Errorf("into the default list = %+v, %v", made, err)
	}
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpdateAccount(ctx, work.AccountID, store.AccountUpdate{ReadOnly: ptr(true)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: all[0].ID, Completed: ptr(true)}); code(err) != api.CodeConflict {
		t.Errorf("a read-only account: %v", err)
	}
	if err := c.Tasks().Delete(ctx, &api.TasksDeleteParams{ID: 99999}); code(err) != api.CodeNotFound {
		t.Errorf("an unknown task: %v", err)
	}
}
