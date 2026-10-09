package pimsync

import (
	"context"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// indexDAV stores the original source and indexes it according to its collection.
func (p *pass) indexDAV(ctx context.Context, tx *store.Tx, col store.Collection, remote davx.Object) error {
	o := store.Object{CollectionID: col.ID, Href: remote.Href, ETag: remote.ETag, Raw: remote.Data}
	switch col.Kind {
	case api.CollectionKindCalendar:
		return p.indexCalendar(ctx, tx, o)
	case api.CollectionKindTasklist:
		return p.indexTodo(ctx, tx, o)
	}
	o.Kind = store.ObjectVCard
	card, err := vcardx.Parse(o.Raw)
	if err != nil {
		o.ParseError = err.Error()
	} else {
		o.UID = card.UID
	}
	id, err := tx.PutObject(ctx, o)
	if err != nil {
		return fmt.Errorf("store contact object: %w", err)
	}
	if card == nil || card.Kind == "group" {
		return tx.RemoveContact(ctx, id)
	}
	c := store.ContactIndex{DisplayName: card.DisplayName(), SortKey: card.SortKey(),
		GivenName: card.GivenName, FamilyName: card.FamilyName, Organization: card.Organization}
	for _, email := range card.Emails {
		c.Emails = append(c.Emails, store.ContactEmail{Email: email.Value, Label: email.Label})
	}
	if len(card.Photo.Data) > 0 {
		c.Photo, c.PhotoType = card.Photo.Data, card.Photo.Type
	}
	return tx.IndexContact(ctx, id, c)
}

func calendarMetadata(o *store.Object) {
	o.Kind = store.ObjectOther
	components, err := contentline.Parse(o.Raw)
	if err != nil {
		o.ParseError = err.Error()
		return
	}
	// Events take priority over tasks, regardless of their order in the source.
	for _, name := range []string{"VEVENT", "VTODO"} {
		for _, calendar := range components {
			if calendar.Name != "VCALENDAR" {
				continue
			}
			children := calendar.ChildrenNamed(name)
			if len(children) == 0 {
				continue
			}
			o.Kind = store.ObjectVEvent
			if name == "VTODO" {
				o.Kind = store.ObjectVTodo
			}
			if uid := children[0].Prop("UID"); uid != nil {
				o.UID = strings.TrimSpace(uid.Text())
			}
			return
		}
	}
}

// indexTodo keeps every source and indexes only a task list's valid VTODOs.
func (p *pass) indexTodo(ctx context.Context, tx *store.Tx, o store.Object) error {
	calendarMetadata(&o)
	var todo calendar.Todo
	if o.Kind == store.ObjectVTodo {
		var err error
		todo, err = calendar.ParseTodo(o.Raw, calendar.Options{Local: p.m.cfg.Local})
		if err != nil {
			o.Kind, o.ParseError = store.ObjectOther, err.Error()
		}
	}
	id, err := tx.PutObject(ctx, o)
	if err != nil {
		return fmt.Errorf("store task object: %w", err)
	}
	if o.Kind != store.ObjectVTodo {
		return tx.RemoveTask(ctx, id)
	}
	row := store.TaskRow{UID: todo.UID, ParentUID: todo.ParentUID, Title: todo.Summary,
		Notes: todo.Description, Due: todo.Due, Completed: todo.Completed, Position: todo.SortOrder}
	if !todo.CompletedAt.IsZero() {
		row.CompletedAt = &todo.CompletedAt
	}
	return tx.IndexTask(ctx, id, row)
}
