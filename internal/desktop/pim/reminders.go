package pim

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
)

const remindersKey = "reminders"

// remindersState is the reminders window's.
type remindersState struct {
	open    bool
	had     bool
	list    fetch[[]api.Reminder]
	pending map[string]bool
}

func (m *Module) remindersEvent(count int64) {
	m.remind.list.dirty = true
	if count > 0 {
		m.raiseReminders()
	}
}

func (m *Module) remindersChanged() { m.remind.list.dirty = true }

// raiseReminders shows the reminders window, or raises it.
func (m *Module) raiseReminders() {
	if m.o.OpenWindow == nil {
		return
	}
	r := &m.remind
	if r.pending == nil {
		r.pending = map[string]bool{}
	}
	if !r.open {
		r.list.started, r.had = false, false
	}
	r.open = true
	m.o.OpenWindow(WindowSpec{Key: remindersKey, Title: "Reminders", Width: 380, Height: 300, MinWidth: 380, MinHeight: 300, AlwaysOnTop: true, Frameless: true,
		View: m.remindersView, OnClose: func() { m.remind.open = false }})
}

func (m *Module) closeReminders() {
	m.remind.open = false
	if m.o.CloseWindow != nil {
		m.o.CloseWindow(remindersKey)
	}
}

// reminderRows are the rows not being answered.
func (m *Module) reminderRows() []api.Reminder {
	var out []api.Reminder
	for _, r := range m.remind.list.value {
		if !m.remind.pending[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

func (m *Module) remindersView(c *ui.Context) {
	frostui.Adapt(c)
	m.frame(c)
	t := c.Theme()
	r := &m.remind
	m.loadCalendars()
	want(m, &r.list, "reminders", func(ctx context.Context, c *api.Client) ([]api.Reminder, error) {
		return c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
	})
	rows := m.reminderRows()
	if len(rows) > 0 {
		r.had = true
	} else if r.had && len(r.pending) == 0 { // an answer can still fail and bring its row back
		r.had = false
		m.closeReminders()
	}
	c.After(30 * time.Second)
	c.OnWindowShortcut(0, ui.KeyEscape, m.closeReminders)
	ui.Column(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Role(ui.RoleDialog).Label("Reminders").Children(func() {
		ui.Row(c).Height(36).Shrink(0).Padding(0, frostui.SpaceM).AlignItems(ui.Center).DragWindow().Children(func() {
			frostui.Strong(c, "Reminders").Grow(1)
			if frostui.IconButton(c, "window-close", "Close").Clicked() {
				m.closeReminders()
			}
		})
		frostui.RowDivider(c)
		ui.Scroll(c).Grow(1).Fill().Children(func() {
			ui.Column(c).AlignItems(ui.Stretch).Role(ui.RoleList).Label("Reminders").Children(func() {
				for _, row := range rows {
					m.reminderRow(c.Key(row.ID), row)
				}
			})
		})
		frostui.RowDivider(c)
		ui.Row(c).Height(44).Shrink(0).Padding(0, frostui.SpaceM).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if len(rows) >= 2 && ui.Button(c, "Dismiss All").Clicked() {
				ids := make([]string, len(rows))
				for i, x := range rows {
					ids[i] = x.ID
				}
				m.answerReminders(ids, nil)
			}
		})
	})
}

func (m *Module) reminderRow(c *ui.Context, r api.Reminder) {
	t := c.Theme()
	name := r.Summary
	if name == "" {
		name = "No Title"
	}
	when := m.clock.reminderWhen(r, m.now())
	if r.Location != "" {
		when += " · " + r.Location
	}
	color := m.colorOf(t, r.CalendarID)
	ui.Row(c).Height(56).Padding(0, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Center).Role(ui.RoleListItem).
		Draw(func(p *ui.Painter, b ui.Rect) { p.Fill(ui.Rect{X: b.X, Y: b.Y + 12, W: 4, H: b.H - 24}, color, 2) }).
		Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Start).Children(func() {
				if textButton(c, name, t.Text, 13).FontWeight(600).Clicked() {
					m.openReminder(r)
				}
				frostui.Muted(c, when).SingleLine()
			})
			ui.MenuButton(c, "Snooze", func(menu *ui.Menu) {
				for _, ch := range snoozeChoices(m.now(), m.clock.loc) {
					if menu.Item(ch.label).Chosen() {
						until := ch.until
						m.answerReminders([]string{r.ID}, &until)
					}
				}
			}).Height(28).Label("Snooze")
			if frostui.IconButton(c, "object-select", "Dismiss").Clicked() {
				m.answerReminders([]string{r.ID}, nil)
			}
		})
}

// openReminder shows the occurrence in Calendar, keeping the reminder.
func (m *Module) openReminder(r api.Reminder) {
	date := r.StartDate
	if !r.AllDay {
		date, _ = zoned(r.Start, m.clock.loc)
	}
	m.cal.selected = &occKey{r.EventID, r.RecurrenceID}
	m.cal.date = date
	m.show(commands.Calendar)
}

// answerReminders snoozes or dismisses; the rows leave at once, and come
// back with a notice if maild refuses.
func (m *Module) answerReminders(ids []string, until *time.Time) {
	c := m.client
	if c == nil {
		return
	}
	for _, id := range ids {
		m.remind.pending[id] = true
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		var err error
		if until != nil {
			err = c.Calendar().Snooze(ctx, &api.CalendarSnoozeParams{IDs: ids, Until: *until})
		} else {
			err = c.Calendar().Dismiss(ctx, &api.CalendarDismissParams{IDs: ids})
		}
		if err != nil {
			slog.Warn("change reminders", "err", err)
		}
		m.o.Post(func() {
			r := &m.remind
			for _, id := range ids {
				delete(r.pending, id)
			}
			// A calendar.reminders call still out was asked before this
			// answer, and would bring the answered rows back: its answer no
			// longer counts, and the next frame asks again.
			r.list.gen++
			r.list.dirty = true
			if err != nil {
				verb := "dismiss"
				if until != nil {
					verb = "snooze"
				}
				m.o.Toast("Could not "+verb+" the reminder", err)
				return
			}
			r.list.value = slices.DeleteFunc(slices.Clone(r.list.value), func(x api.Reminder) bool { return slices.Contains(ids, x.ID) })
		})
	}()
}
