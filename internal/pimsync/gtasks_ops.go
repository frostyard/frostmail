package pimsync

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/store"
)

// googleTasksClient is the Tasks API client of a pass's account.
func (p *pass) googleTasksClient(s store.Service) *gtasks.Client {
	return gtasks.New(s.URL, gtasks.Options{HTTP: p.m.cfg.HTTP, UserAgent: p.m.cfg.UserAgent,
		Token: func(ctx context.Context) (string, error) {
			if p.m.cfg.Tokens == nil {
				return "", fmt.Errorf("%w: maild has no OAuth sign-in", errSignIn)
			}
			return p.m.cfg.Tokens.AccessToken(ctx, p.acct.ID)
		}})
}

// replayGoogleTask writes one tasks.insert, tasks.patch or tasks.delete
// (docs/design/pim.md, Tasks: Writes). Changes made to a task before Google
// had it name its object, whose href is its Google ID by the time they run.
func (p *pass) replayGoogleTask(ctx context.Context, op store.PIMOp, col store.Collection, services []store.Service) error {
	svc, ok := enabledService(services, api.ServiceKindTasks)
	if !ok || p.scope(api.ServiceKindTasks) != nil {
		return nil // waits until the service is on and signed in
	}
	c := p.googleTasksClient(svc)
	if op.Kind == "tasks.delete" {
		err := c.Delete(ctx, col.Href, op.Href)
		if err != nil && !errors.Is(err, gtasks.ErrNotFound) {
			return p.failed(ctx, op, err)
		}
		return p.done(ctx, op, nil)
	}
	if op.ObjectID == nil {
		return p.done(ctx, op, nil)
	}
	obj, err := p.m.db.GetObject(ctx, *op.ObjectID)
	if errors.Is(err, store.ErrNotFound) {
		return p.done(ctx, op, nil)
	}
	if err != nil {
		return err
	}
	var change store.TaskChange
	if err := json.Unmarshal([]byte(op.Payload), &change); err != nil {
		return p.failed(ctx, op, fmt.Errorf("pimsync: task change: %w", err))
	}
	fields := gtasks.Fields{Title: change.Title, Notes: change.Notes, Due: change.Due, Completed: change.Completed}
	var task gtasks.Task
	switch op.Kind {
	case "tasks.insert":
		var parent string
		if parent, err = p.parentHref(ctx, change.Parent); err != nil {
			return err
		}
		task, err = c.Insert(ctx, col.Href, fields, parent, "")
	case "tasks.patch":
		task, err = c.Patch(ctx, col.Href, obj.Href, fields)
		if errors.Is(err, gtasks.ErrNotFound) {
			// Deleted on the server: so it is here.
			return p.done(ctx, op, func(tx *store.Tx) error {
				_, err := tx.DeleteObjects(ctx, col.ID, []string{obj.Href})
				return err
			})
		}
	default:
		return p.failed(ctx, op, fmt.Errorf("pimsync: no replay for %q", op.Kind))
	}
	if err != nil {
		return p.failed(ctx, op, err)
	}
	return p.done(ctx, op, func(tx *store.Tx) error {
		if obj.Href != task.ID {
			if err := tx.SetObjectHref(ctx, obj.ID, task.ID); err != nil {
				return err
			}
			if err := tx.ReparentTasks(ctx, col.ID, obj.Href, task.ID); err != nil {
				return err
			}
		}
		if err := indexGoogleTask(ctx, tx, col.ID, task); err != nil {
			return err
		}
		return tx.Emit(ctx, api.TasksChanged{AccountID: p.acct.ID})
	})
}

// parentHref is the Google ID of a new subtask's parent, or "" when it has
// none or the parent is gone.
func (p *pass) parentHref(ctx context.Context, parent int64) (string, error) {
	if parent == 0 {
		return "", nil
	}
	obj, err := p.m.db.GetObject(ctx, parent)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return obj.Href, nil
}
