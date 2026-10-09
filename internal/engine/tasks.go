package engine

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// Tasks implements the tasks domain (docs/design/pim.md, Tasks). Changes
// apply to the store at once and wait in pim_ops for pimsync to write
// them: tasks.insert, tasks.patch and tasks.delete for Google lists.
func (e *Engine) Tasks() api.TasksService { return tasksService{e.d} }

type tasksService struct{ Deps }

// localPrefix marks the href of a task created here until Google gives it
// an ID.
const localPrefix = "local-"

func (t tasksService) List(ctx context.Context, p *api.TasksListParams) ([]api.Task, error) {
	var f store.TaskFilter
	if p.ListID != nil {
		col, err := t.DB.GetCollection(ctx, *p.ListID)
		if err != nil || col.Kind != api.CollectionKindTasklist {
			return nil, api.NotFound("task list %d does not exist", *p.ListID)
		}
		f.ListID = col.ID
	}
	if p.Completed != nil {
		f.Completed = *p.Completed
	}
	if p.DueBefore != nil {
		if !validDate(*p.DueBefore) {
			return nil, api.InvalidParams("dueBefore is YYYY-MM-DD")
		}
		f.DueBefore = *p.DueBefore
	}
	rows, err := t.DB.Tasks(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]api.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, t.task(ctx, r))
	}
	return out, nil
}

func (t tasksService) task(ctx context.Context, r store.TaskRow) api.Task {
	a := api.Task{ID: r.ObjectID, ListID: r.ListID, AccountID: r.AccountID, Title: r.Title, Notes: r.Notes, Due: r.Due,
		Completed: r.Completed, CompletedAt: r.CompletedAt, ReadOnly: r.ReadOnly}
	if r.ParentID != 0 {
		parent := r.ParentID
		a.ParentID = &parent
	}
	if r.GmThrID != 0 {
		if id, err := t.DB.GmailThreadMessage(ctx, r.AccountID, r.GmThrID); err == nil {
			a.MessageID = &id
		}
	}
	return a
}

func validDate(s string) bool {
	d, err := time.Parse(time.DateOnly, s)
	return err == nil && d.Format(time.DateOnly) == s
}

// writableList returns a task list Frostmail can change: a Google list of
// an account that is not read-only.
func (t tasksService) writableList(ctx context.Context, id int64) (store.Collection, error) {
	col, err := t.DB.GetCollection(ctx, id)
	if err != nil || col.Kind != api.CollectionKindTasklist {
		return col, api.NotFound("task list %d does not exist", id)
	}
	acct, err := t.DB.GetAccount(ctx, col.AccountID)
	if err != nil {
		return col, err
	}
	if col.ReadOnly || acct.ReadOnly {
		return col, api.Conflict("the task list %q is read-only", col.Name)
	}
	if providers.ForKind(acct.Kind).DAV.Tasks != "google" {
		return col, api.Conflict("tasks in CalDAV lists cannot be changed yet")
	}
	return col, nil
}

// defaultList is the default list of the first account with tasks on, else
// its first list.
func (t tasksService) defaultList(ctx context.Context) (int64, error) {
	rows, err := t.DB.Collections(ctx, store.CollectionFilter{Kind: api.CollectionKindTasklist})
	if err != nil {
		return 0, err
	}
	services, err := t.DB.Services(ctx, 0)
	if err != nil {
		return 0, err
	}
	on := map[int64]bool{}
	for _, s := range services {
		on[s.AccountID] = on[s.AccountID] || (s.Service == api.ServiceKindTasks && s.Enabled)
	}
	var first int64
	for _, c := range rows {
		if !on[c.AccountID] || !c.Enabled {
			continue
		}
		if c.IsDefault {
			return c.ID, nil
		}
		if first == 0 {
			first = c.ID
		}
	}
	if first == 0 {
		return 0, api.NotFound("no task list to add to")
	}
	return first, nil
}

func (t tasksService) Create(ctx context.Context, p *api.TasksCreateParams) (*api.Task, error) {
	change, err := createChange(p)
	if err != nil {
		return nil, err
	}
	listID := int64(0)
	if p.ListID != nil {
		listID = *p.ListID
	} else if listID, err = t.defaultList(ctx); err != nil {
		return nil, err
	}
	col, err := t.writableList(ctx, listID)
	if err != nil {
		return nil, err
	}
	var parent store.TaskRow
	if p.ParentID != nil {
		if parent, err = t.DB.Task(ctx, *p.ParentID); err != nil {
			return nil, apiError(err, "the parent task")
		}
		if parent.ListID != col.ID || parent.ParentID != 0 {
			return nil, api.InvalidParams("a subtask's parent is a top-level task of the same list")
		}
		change.Parent = parent.ObjectID
	}
	id, err := t.createLocal(ctx, col, change, parent.UID)
	if err != nil {
		return nil, err
	}
	return t.after(ctx, col.AccountID, id)
}

func createChange(p *api.TasksCreateParams) (store.TaskChange, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return store.TaskChange{}, api.InvalidParams("a task needs a title")
	}
	c := store.TaskChange{Title: &title, Notes: p.Notes}
	if p.Due != nil && *p.Due != "" {
		if !validDate(*p.Due) {
			return c, api.InvalidParams("due is YYYY-MM-DD")
		}
		c.Due = p.Due
	}
	return c, nil
}

// createLocal stores a new task under a local href, at the top of its
// list, and queues its insert.
func (t tasksService) createLocal(ctx context.Context, col store.Collection, c store.TaskChange, parentUID string) (int64, error) {
	uid, err := newUUID()
	if err != nil {
		return 0, err
	}
	href := localPrefix + uid
	raw, err := json.Marshal(applyChange(map[string]any{"status": "needsAction"}, c, time.Time{}))
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return 0, err
	}
	var id int64
	err = t.DB.Tx(ctx, func(tx *store.Tx) error {
		if id, err = tx.PutObject(ctx, store.Object{CollectionID: col.ID, Href: href, Kind: store.ObjectGTask, UID: href, Raw: raw}); err != nil {
			return err
		}
		row := store.TaskRow{UID: href, ParentUID: parentUID, Title: *c.Title, Position: localPosition(t.DB.Now())}
		if c.Notes != nil {
			row.Notes = *c.Notes
		}
		if c.Due != nil {
			row.Due = *c.Due
		}
		if err := tx.IndexTask(ctx, id, row); err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &id,
			Kind: "tasks.insert", Href: href, Payload: string(payload)}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.TasksChanged{AccountID: col.AccountID})
	})
	return id, err
}

// localPosition puts a task made here at the top of its list, as Google
// will: "0/" sorts before Google's zero-padded positions, and later tasks
// before earlier ones.
func localPosition(now time.Time) string {
	return fmt.Sprintf("0/%019d", math.MaxInt64-now.UnixNano())
}

// applyChange sets a task's JSON fields as Google would.
func applyChange(task map[string]any, c store.TaskChange, now time.Time) map[string]any {
	if c.Title != nil {
		task["title"] = *c.Title
	}
	if c.Notes != nil {
		task["notes"] = *c.Notes
	}
	if c.Due != nil {
		if *c.Due == "" {
			delete(task, "due")
		} else {
			task["due"] = *c.Due + "T00:00:00.000Z"
		}
	}
	if c.Completed != nil {
		if *c.Completed {
			task["status"] = "completed"
			task["completed"] = now.UTC().Format("2006-01-02T15:04:05.000Z")
		} else {
			task["status"] = "needsAction"
			delete(task, "completed")
		}
	}
	return task
}

// after kicks the account's sync and answers the task as stored.
func (t tasksService) after(ctx context.Context, accountID, id int64) (*api.Task, error) {
	if t.PIM != nil {
		t.PIM.Kick(accountID)
	}
	row, err := t.DB.Task(ctx, id)
	if err != nil {
		return nil, err
	}
	out := t.task(ctx, row)
	return &out, nil
}

func (t tasksService) Update(ctx context.Context, p *api.TasksUpdateParams) (*api.Task, error) {
	row, err := t.DB.Task(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, "the task")
	}
	col, err := t.writableList(ctx, row.ListID)
	if err != nil {
		return nil, err
	}
	c, err := updateChange(p)
	if err != nil {
		return nil, err
	}
	obj, err := t.DB.GetObject(ctx, row.ObjectID)
	if err != nil {
		return nil, err
	}
	task := map[string]any{}
	if err := json.Unmarshal(obj.Raw, &task); err != nil {
		return nil, fmt.Errorf("task %d's source: %w", row.ObjectID, err)
	}
	now := t.DB.Now()
	if obj.Raw, err = json.Marshal(applyChange(task, c, now)); err != nil {
		return nil, err
	}
	row = patchRow(row, c, now)
	payload, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	err = t.DB.Tx(ctx, func(tx *store.Tx) error {
		if _, err := tx.PutObject(ctx, obj); err != nil {
			return err
		}
		if err := tx.IndexTask(ctx, row.ObjectID, row); err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &row.ObjectID,
			Kind: "tasks.patch", Href: obj.Href, Payload: string(payload)}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.TasksChanged{AccountID: col.AccountID})
	})
	if err != nil {
		return nil, err
	}
	return t.after(ctx, col.AccountID, row.ObjectID)
}

func updateChange(p *api.TasksUpdateParams) (store.TaskChange, error) {
	c := store.TaskChange{Notes: p.Notes, Completed: p.Completed}
	if p.Title != nil {
		title := strings.TrimSpace(*p.Title)
		if title == "" {
			return c, api.InvalidParams("a task needs a title")
		}
		c.Title = &title
	}
	if p.Due != nil {
		if *p.Due != "" && !validDate(*p.Due) {
			return c, api.InvalidParams("due is YYYY-MM-DD, or empty to clear it")
		}
		c.Due = p.Due
	}
	return c, nil
}

func patchRow(row store.TaskRow, c store.TaskChange, now time.Time) store.TaskRow {
	if c.Title != nil {
		row.Title = *c.Title
	}
	if c.Notes != nil {
		row.Notes = *c.Notes
	}
	if c.Due != nil {
		row.Due = *c.Due
	}
	if c.Completed != nil {
		row.Completed, row.CompletedAt = *c.Completed, nil
		if *c.Completed {
			at := now.UTC()
			row.CompletedAt = &at
		}
	}
	return row
}

func (t tasksService) Delete(ctx context.Context, p *api.TasksDeleteParams) error {
	row, err := t.DB.Task(ctx, p.ID)
	if err != nil {
		return apiError(err, "the task")
	}
	col, err := t.writableList(ctx, row.ListID)
	if err != nil {
		return err
	}
	all, err := t.DB.Tasks(ctx, store.TaskFilter{ListID: col.ID, Completed: true})
	if err != nil {
		return err
	}
	gone := []int64{row.ObjectID}
	for _, r := range all {
		if r.ParentID == row.ObjectID {
			gone = append(gone, r.ObjectID)
		}
	}
	err = t.DB.Tx(ctx, func(tx *store.Tx) error { return t.deleteLocal(ctx, tx, col, gone) })
	if err != nil {
		return err
	}
	if t.PIM != nil {
		t.PIM.Kick(col.AccountID)
	}
	return nil
}

// deleteLocal deletes a task and its subtasks here, and queues a
// tasks.delete for the first (Google deletes the subtasks with it) unless
// Google never had it.
func (t tasksService) deleteLocal(ctx context.Context, tx *store.Tx, col store.Collection, ids []int64) error {
	var hrefs []string
	for i, id := range ids {
		obj, err := t.DB.GetObject(ctx, id)
		if err != nil {
			return err
		}
		hrefs = append(hrefs, obj.Href)
		if strings.HasPrefix(obj.Href, localPrefix) {
			if _, err := tx.DropPIMOps(ctx, id); err != nil {
				return err
			}
			continue
		}
		if i == 0 {
			if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID,
				Kind: "tasks.delete", Href: obj.Href}); err != nil {
				return err
			}
		}
	}
	if _, err := tx.DeleteObjects(ctx, col.ID, hrefs); err != nil {
		return err
	}
	return tx.Emit(ctx, api.TasksChanged{AccountID: col.AccountID})
}
