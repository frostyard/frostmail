package pimsync

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/store"
)

func (p *pass) syncGoogleTasks(ctx context.Context, s store.Service) error {
	c := p.googleTasksClient(s)
	lists, err := c.Lists(ctx)
	if err != nil {
		return fmt.Errorf("list google task lists: %w", err)
	}
	remote := make([]store.RemoteCollection, 0, len(lists))
	for _, list := range lists {
		remote = append(remote, store.RemoteCollection{Href: list.ID, Name: list.Title})
	}
	var cols []store.Collection
	if err := p.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		cols, err = tx.ReplaceCollections(ctx, p.acct.ID, api.CollectionKindTasklist, remote)
		return err
	}); err != nil {
		return fmt.Errorf("replace google task lists: %w", err)
	}
	var first error
	for _, col := range cols {
		if !col.Enabled {
			continue
		}
		if err := p.syncGoogleTaskList(ctx, c, col); err != nil {
			err = fmt.Errorf("sync google task list %q: %w", col.Href, err)
			if errors.Is(err, gtasks.ErrUnauthorized) {
				return err
			}
			if first == nil {
				first = err
			}
		}
	}
	return first
}

func (p *pass) syncGoogleTaskList(ctx context.Context, c *gtasks.Client, col store.Collection) error {
	var since time.Time
	if col.SyncToken != "" {
		var err error
		since, err = time.Parse(time.RFC3339Nano, col.SyncToken)
		if err != nil {
			return fmt.Errorf("parse tasks sync token: %w", err)
		}
	}
	tasks, err := c.Tasks(ctx, col.Href, since)
	if err != nil {
		return fmt.Errorf("fetch google tasks: %w", err)
	}
	newest := since
	for _, task := range tasks {
		if task.Updated.After(newest) {
			newest = task.Updated
		}
	}
	token := col.SyncToken
	if newest.After(since) {
		token = newest.UTC().Format(time.RFC3339Nano)
	}
	return p.m.db.Tx(ctx, func(tx *store.Tx) error {
		changed, err := p.googleTaskChanges(ctx, col.ID, tasks)
		if err != nil {
			return err
		}
		count := 0
		for _, task := range changed {
			if task.Deleted {
				n, err := tx.DeleteObjects(ctx, col.ID, []string{task.ID})
				if err != nil {
					return err
				}
				count += n
			} else {
				if err := indexGoogleTask(ctx, tx, col.ID, task); err != nil {
					return err
				}
				count++
			}
		}
		if count > 0 {
			if err := tx.Emit(ctx, api.TasksChanged{AccountID: p.acct.ID}); err != nil {
				return err
			}
		}
		return tx.SetCollectionSync(ctx, col.ID, token, col.CTag)
	})
}

// googleTaskChanges leaves pending local edits and unchanged source alone.
func (p *pass) googleTaskChanges(ctx context.Context, collectionID int64, tasks []gtasks.Task) ([]gtasks.Task, error) {
	pending, err := p.m.db.PendingHrefs(ctx, collectionID)
	if err != nil {
		return nil, fmt.Errorf("read pending tasks: %w", err)
	}
	var changed []gtasks.Task
	for _, task := range tasks {
		if pending[task.ID] {
			continue
		}
		old, err := p.m.db.ObjectByHref(ctx, collectionID, task.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("read google task %q: %w", task.ID, err)
		}
		if task.Deleted || errors.Is(err, store.ErrNotFound) || !bytes.Equal(old.Raw, task.Raw) {
			changed = append(changed, task)
		}
	}
	return changed, nil
}

func indexGoogleTask(ctx context.Context, tx *store.Tx, collectionID int64, task gtasks.Task) error {
	var meta struct {
		ETag string `json:"etag"`
	}
	if err := json.Unmarshal(task.Raw, &meta); err != nil {
		return fmt.Errorf("read google task etag: %w", err)
	}
	id, err := tx.PutObject(ctx, store.Object{CollectionID: collectionID, Kind: store.ObjectGTask,
		Href: task.ID, UID: task.ID, Raw: task.Raw, ETag: meta.ETag})
	if err != nil {
		return fmt.Errorf("store google task: %w", err)
	}
	r := store.TaskRow{UID: task.ID, ParentUID: task.Parent, Title: task.Title, Notes: task.Notes,
		Due: task.Due, Completed: task.Completed, Position: task.Position, GmThrID: googleTaskThread(task.Links)}
	if !task.CompletedAt.IsZero() {
		at := task.CompletedAt.UTC()
		r.CompletedAt = &at
	}
	return tx.IndexTask(ctx, id, r)
}

func googleTaskThread(links []gtasks.Link) int64 {
	for _, link := range links {
		if link.Type != "email" {
			continue
		}
		u, err := url.Parse(link.Link)
		if err != nil {
			return 0
		}
		threadPath := u.Path
		if u.Fragment != "" {
			threadPath = u.Fragment
		}
		id, err := strconv.ParseUint(path.Base(threadPath), 16, 64)
		if err != nil {
			return 0
		}
		return int64(id)
	}
	return 0
}
