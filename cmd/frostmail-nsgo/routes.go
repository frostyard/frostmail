package main

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

func eventPath(e api.CalendarEvent) string {
	q := url.Values{}
	if e.ID > 0 {
		path := fmt.Sprintf("/event/%d", e.ID)
		if e.RecurrenceID != "" {
			q.Set("recurrenceId", e.RecurrenceID)
			path += "?" + q.Encode()
		}
		return path
	}
	if e.CalendarID > 0 {
		q.Set("calendarId", strconv.FormatInt(e.CalendarID, 10))
	}
	if e.AllDay {
		q.Set("allDay", "1")
		q.Set("startDate", e.StartDate)
		q.Set("endDate", e.EndDate)
	} else if !e.Start.IsZero() {
		q.Set("start", e.Start.Format(time.RFC3339))
		q.Set("end", e.End.Format(time.RFC3339))
	}
	if len(q) == 0 {
		return "/event/new"
	}
	return "/event/new?" + q.Encode()
}

type location struct {
	path, place, title string
	depth              bool
}

func (w *workspace) location() location {
	if p := w.overlay.Get(); p != nil && p.route != "" {
		return location{path: p.route, place: p.route, title: map[string]string{"compose": "Compose", "event": "Event", "reminders": "Reminders"}[strings.Split(p.route, "/")[1]]}
	}
	if m := w.module.Get(); m != "Mail" {
		return location{path: "/" + strings.ToLower(m), place: m, title: m}
	}
	w.selected.Get()
	source := w.source.Key()
	if w.source.Kind == mail.VIP {
		source = "vips"
	}
	path := ""
	if source != "all-inboxes" {
		path = "/" + strings.Replace(source, ":", "/", 1)
	}
	id := w.opened.Get()
	if id > 0 {
		path += fmt.Sprintf("/message/%d", id)
	}
	if path == "" {
		path = "/"
	}
	return location{path: path, place: source, title: w.title(), depth: id > 0 && w.cx.Env().Window().Width < 800}
}

// applyRoute is called on the UI thread after the daemon is ready. The
// completion releases the history reporter after asynchronous lookups settle.
func (w *workspace) applyRoute(raw string, done func()) {
	w.routeGeneration++
	generation := w.routeGeneration
	run := func() {
		if generation != w.routeGeneration {
			return
		}
		w.closePanel()
		u, err := url.ParseRequestURI(raw)
		if err != nil {
			w.problem.Set("Invalid mail link")
			done()
			return
		}
		path := strings.Trim(u.EscapedPath(), "/")
		seg := strings.Split(path, "/")
		idOf := func(value string) int64 {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 1 {
				return 0
			}
			return n
		}
		for _, m := range []string{"Settings", "People", "Calendar", "Tasks", "Activity"} {
			if path == strings.ToLower(m) {
				w.module.Set(m)
				done()
				return
			}
		}
		if path == "reminders" {
			w.reminders()
			done()
			return
		}
		if len(seg) == 2 && seg[0] == "compose" && idOf(seg[1]) > 0 {
			id := idOf(seg[1])
			perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
				return c.Draft().Get(ctx, &api.DraftGetParams{ID: id})
			}, func(d *api.Draft, err error) {
				if generation == w.routeGeneration {
					w.editDraft(d, err)
					done()
				}
			})
			return
		}
		if len(seg) == 2 && seg[0] == "event" {
			w.module.Set("Calendar")
			q := u.Query()
			if seg[1] == "new" {
				start, _ := time.Parse(time.RFC3339, q.Get("start"))
				end, _ := time.Parse(time.RFC3339, q.Get("end"))
				w.eventForm(api.CalendarEvent{Start: start, End: end, AllDay: q.Get("allDay") == "1", StartDate: q.Get("startDate"), EndDate: q.Get("endDate"), CalendarID: idOf(q.Get("calendarId"))}, nil, func() {})
				done()
				return
			}
			if id := idOf(seg[1]); id > 0 {
				perform(w, func(ctx context.Context, c *api.Client) (*api.CalendarEvent, error) {
					return c.Calendar().Event(ctx, &api.CalendarEventParams{ID: id, RecurrenceID: changed("", q.Get("recurrenceId"))})
				}, func(e *api.CalendarEvent, err error) {
					if generation != w.routeGeneration {
						return
					}
					if err != nil {
						w.problem.Set(err.Error())
					} else {
						w.eventDetail(*e, func() {})
					}
					done()
				})
				return
			}
		}
		message := int64(0)
		if n := len(seg); n >= 2 && seg[n-2] == "message" {
			message = idOf(seg[n-1])
			if n == 2 && message == 0 {
				value, err := url.PathUnescape(seg[1])
				if err == nil && value != "" {
					perform(w, func(ctx context.Context, c *api.Client) (*api.MessageSummary, error) {
						return c.Message().ByMessageID(ctx, &api.MessageByMessageIDParams{MessageID: value})
					}, func(m *api.MessageSummary, err error) {
						if generation != w.routeGeneration {
							return
						}
						if err != nil {
							w.problem.Set(err.Error())
						} else {
							source := mail.Source{Kind: mail.AllInboxes}
							for _, box := range w.store.State().Mailboxes {
								if slices.Contains(m.MailboxIDs, box.ID) && (source.ID == 0 || box.Role == api.MailboxRoleInbox) {
									source = mail.Source{Kind: mail.Mailbox, ID: box.ID}
								}
							}
							w.navigate(source.Key())
							w.openMessage(m.ID, false)
						}
						done()
					})
					return
				}
			}
			seg = seg[:n-2]
		}
		source, ok := mail.SourceFromKey(strings.Join(seg, ":"), nil)
		if !ok {
			source = mail.Source{Kind: mail.AllInboxes}
		}
		w.navigate(source.Key())
		if message > 0 {
			w.openMessage(message, false)
		}
		done()
	}
	if save := w.beforeRoute; save != nil {
		save(run, done)
	} else {
		run()
	}
}
