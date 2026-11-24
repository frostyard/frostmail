package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/frostyard/frostmail/api"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"strconv"
	"strings"
)

func (w *workspace) whenConnected(f func()) {
	if w.session.Client() != nil && w.pendingTarget == "" {
		f()
	} else {
		w.deferredOpen = f
	}
}
func (w *workspace) notificationKey() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(w.session.Target().Key())))
}
func (w *workspace) commands() {
	cx := w.cx
	cx.OnKey("mod+shift+p", w.palette)
	for _, entry := range w.commandTable() {
		if entry.key != "" {
			cx.OnKey(entry.key, func() {
				if w.overlay.GetUntracked() == nil && (entry.enabled == nil || entry.enabled()) {
					entry.run()
				}
			})
		}
	}
	cx.OnNotificationPress(func(key string) {
		origin, arg, ok := strings.Cut(key, "/")
		if !ok || origin != w.notificationKey() {
			return
		}
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return
		}
		cx.OrderFront()
		w.whenConnected(func() {
			if origin != w.notificationKey() {
				return
			}
			w.module.Set("Mail")
			if id > 0 {
				w.openMessage(id, false)
			} else {
				w.navigate("all-inboxes")
			}
		})
	})
	cx.Cleanup(w.events.On(func(ev api.Event) {
		target := w.session.Target()
		switch n := ev.(type) {
		case api.PeopleChanged:
			cx.Post(func() {
				if c := w.session.Client(); c != nil && w.session.Target().Same(target) {
					w.resetPhotos(c)
				}
			})
			return
		case api.CalendarReminders:
			cx.Post(func() {
				if w.session.Target().Same(target) {
					w.reminderNotice(n.Count)
				}
			})
			return
		}
		n, ok := ev.(api.MessageAnnounced)
		if !ok {
			return
		}
		cx.Post(func() {
			if !w.session.Target().Same(target) {
				return
			}
			for _, m := range n.Notes {
				id := int64(0)
				if m.MessageID != nil {
					id = *m.MessageID
				}
				_ = cx.ShowNotification(core.Notification{ID: w.notificationKey() + "/" + strconv.FormatInt(id, 10), Title: m.Title, Body: m.Body}, nil)
			}
		})
	}))
}
func (w *workspace) notificationSettings(cx *core.Scope) []core.Node {
	p := core.NewSignal(cx, core.NotificationsUnavailable)
	cx.NotificationPermission(func(state core.NotificationPermission) { cx.Post(func() { p.Set(state) }) })
	return []core.Node{core.Dynamic(func(*core.Scope) core.Node {
		switch p.Get() {
		case core.NotificationsNotAsked:
			return button("Allow notifications on this device", func() { cx.RequestNotificationPermission(p.Set) })
		case core.NotificationsGranted:
			return core.Text{Content: "Notifications are enabled on this device."}
		case core.NotificationsDenied:
			return core.Text{Content: "Notifications are blocked by the system."}
		}
		return nil
	})}
}

func (w *workspace) checkReminders() {
	perform(w, func(ctx context.Context, c *api.Client) ([]api.Reminder, error) {
		return c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
	}, func(rows []api.Reminder, err error) {
		if err == nil {
			w.reminderNotice(int64(len(rows)))
		}
	})
}
func (w *workspace) reminderNotice(count int64) {
	const id = "calendar-reminders"
	if count == 0 {
		w.toaster.Dismiss(id)
		return
	}
	key := w.notificationKey()
	title := "Calendar reminder"
	if count > 1 {
		title = fmt.Sprintf("%d calendar reminders", count)
	}
	w.toaster.Show(ui.ToastContent{ID: id, Title: title, Action: "View reminders", OnAction: func() {
		if key != w.notificationKey() {
			return
		}
		if save := w.beforeRoute; save != nil {
			save(w.reminders, func() {})
		} else {
			w.reminders()
		}
	}})
}
