package pim

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// alertChoice is one of an event's alert presets.
type alertChoice struct {
	minutes int64
	label   string
}

var (
	timedAlerts = []alertChoice{{0, "At time of event"}, {5, "5 minutes before"}, {10, "10 minutes before"}, {15, "15 minutes before"},
		{30, "30 minutes before"}, {60, "1 hour before"}, {120, "2 hours before"}, {1440, "1 day before"}, {2880, "2 days before"}, {10080, "1 week before"}}
	allDayAlerts = []alertChoice{{-540, "On the day at 9:00 AM"}, {900, "1 day before at 9:00 AM"}, {2340, "2 days before at 9:00 AM"}, {9540, "1 week before at 9:00 AM"}}
	repeatLabels = []struct {
		preset repeatPreset
		label  string
	}{{presetNever, "Never"}, {presetDaily, "Every Day"}, {presetWeekdays, "Every Weekday"}, {presetWeekly, "Every Week"},
		{presetBiweekly, "Every 2 Weeks"}, {presetMonthly, "Every Month"}, {presetYearly, "Every Year"}}
	keyLetters = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// editTarget says what an event window edits: an event, or a new one.
type editTarget struct {
	isNew        bool
	id           int64
	recurrenceID string
	times        newTimes
}

// calendarChoice is a calendar the editor offers.
type calendarChoice struct {
	id          int64
	name, color string
	account     string
	invites     bool
}

// eventWindow is one event's editor.
type eventWindow struct {
	m   *Module
	key string
	// daemon is the maild the window edits on (Module.daemon): its IDs
	// name that maild's events and calendars, and it never writes to
	// another.
	daemon  int
	target  editTarget
	dialogs frostui.Dialogs
	// closed: the window closes without asking about its changes, as after
	// a save, Don't Save or a change of maild.
	closed bool

	failure   string
	loaded    bool
	event     *api.CalendarEvent
	initial   eventDraft
	draft     eventDraft
	calendars []calendarChoice
	readOnly  bool
	zones     []string

	busy        bool
	saveFailure string
	closing     bool
	scope       *scopeSheet
	repeat      *repeatSheet

	startT, endT, untilT time.Time
	inviteeText          string
	suggestions          []api.Address
	suggestToken         int
}

// repeatSheet is the Custom Repeat sheet's draft.
type repeatSheet struct {
	r        repeat
	interval string
}

// every reads the sheet's interval and whether OK can take it. A control
// takes its input as it draws, so call it after the controls.
func (s *repeatSheet) every() (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s.interval))
	return n, err == nil && n >= 1 && (s.r.frequency != "WEEKLY" || len(s.r.weekdays) > 0)
}

func (m *Module) openWindow(w *eventWindow, title string) {
	if m.o.OpenWindow == nil {
		return
	}
	m.windows[w.key] = w
	m.o.OpenWindow(WindowSpec{Key: w.key, Title: title, Width: 480, Height: 600, MinWidth: 420, MinHeight: 480, Frameless: true, View: w.view,
		OnClose: w.forget, CloseRequested: w.closeRequested})
}

// forget takes the window out of the module's, unless another window has
// its key since.
func (w *eventWindow) forget() {
	if w.m.windows[w.key] == w {
		delete(w.m.windows, w.key)
	}
}

// stale reports that the module has left the maild the window edits on.
func (w *eventWindow) stale() bool { return w.daemon != w.m.daemon }

func eventWindowKey(id int64, recurrenceID string) string {
	key := fmt.Sprintf("event-%d", id)
	if recurrenceID != "" {
		key += "-" + strings.Trim(keyLetters.ReplaceAllString(recurrenceID, "-"), "-")
	}
	return key
}

// editSelected opens the selection in its window, or raises the one
// showing it.
func (m *Module) editSelected() {
	sel := m.cal.selected
	if sel == nil {
		return
	}
	if e := m.shownEvent(); e != nil {
		if writable, _ := m.eventRights(e); !writable {
			return
		}
	}
	key := eventWindowKey(sel.eventID, sel.recurrenceID)
	title := "New Event"
	if e := m.shownEvent(); e != nil && e.Summary != "" {
		title = e.Summary
	}
	if w, ok := m.windows[key]; ok {
		m.openWindow(w, title)
		return
	}
	w := &eventWindow{m: m, key: key, daemon: m.daemon, target: editTarget{id: sel.eventID, recurrenceID: sel.recurrenceID}}
	m.openWindow(w, title)
	w.load()
}

// newEventDefault opens a new event on the selected date: at the next
// whole hour today, else at 9:00; an hour long.
func (m *Module) newEventDefault() {
	if len(m.writableCalendars()) == 0 {
		return
	}
	date := m.cal.date
	loc := m.clock.loc
	var start time.Time
	if date == m.today() {
		// Find the next local whole hour, including across offset changes.
		// Absolute-hour truncation lands on :30 or :45 in fractional zones.
		start = m.now().In(loc).Truncate(time.Minute).Add(time.Minute)
		for start.Minute() != 0 {
			start = start.Add(time.Minute)
		}
	} else {
		start = instantAt(date, 9*60, loc)
	}
	m.newEvent(newTimes{start: start, end: start.Add(time.Hour)})
}

func (m *Module) newTimed(r span) { m.newEvent(newTimes{start: r.start, end: r.end}) }

func (m *Module) newAllDay(date string) {
	m.newEvent(newTimes{allDay: true, startDate: date, endDate: addDays(date, 1)})
}

func (m *Module) newEvent(t newTimes) {
	m.newCount++
	w := &eventWindow{m: m, key: fmt.Sprintf("event-new-%d", m.newCount), daemon: m.daemon, target: editTarget{isNew: true, times: t}}
	m.openWindow(w, "New Event")
	w.load()
}

// load fetches what the editor needs.
func (w *eventWindow) load() {
	m := w.m
	c := m.client
	if c == nil {
		w.failure = "Not connected."
		return
	}
	zone, loc := m.zone, m.clock.loc
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		accounts, err := c.Account().List(ctx, &api.AccountListParams{})
		var cols []api.Collection
		if err == nil {
			kind := api.CollectionKindCalendar
			cols, err = c.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
		}
		var e *api.CalendarEvent
		if err == nil && !w.target.isNew {
			p := &api.CalendarEventParams{ID: w.target.id}
			if w.target.recurrenceID != "" {
				p.RecurrenceID = &w.target.recurrenceID
			}
			e, err = c.Calendar().Event(ctx, p)
		}
		m.o.Post(func() { w.loadedWith(accounts, cols, e, err, zone, loc) })
	}()
}

func (w *eventWindow) loadedWith(accounts []api.Account, cols []api.Collection, e *api.CalendarEvent, err error, zone string, loc *time.Location) {
	if err != nil {
		w.failure = err.Error()
		return
	}
	accountOf := func(id int64) *api.Account {
		for i := range accounts {
			if accounts[i].ID == id {
				return &accounts[i]
			}
		}
		return nil
	}
	writable := slices.DeleteFunc(slices.Clone(cols), func(c api.Collection) bool {
		a := accountOf(c.AccountID)
		return c.ReadOnly || a == nil || a.ReadOnly
	})
	choice := func(list []api.Collection) []calendarChoice {
		out := make([]calendarChoice, len(list))
		for i, c := range list {
			email := ""
			if a := accountOf(c.AccountID); a != nil {
				email = a.Email
			}
			out[i] = calendarChoice{c.ID, c.Name, c.Color, email, c.Invites}
		}
		return out
	}
	if w.target.isNew {
		if len(writable) == 0 {
			w.failure = "No writable calendar is available."
			return
		}
		id := writable[0].ID
		for _, c := range writable {
			if c.IsDefault {
				id = c.ID
				break
			}
		}
		w.initial = newDraft(w.target.times, id, zone, loc)
		w.calendars = choice(writable)
	} else {
		w.event = e
		w.readOnly = e.ReadOnly || !slices.ContainsFunc(writable, func(c api.Collection) bool { return c.ID == e.CalendarID })
		w.initial = draftOf(e, zone, loc)
		own := slices.DeleteFunc(slices.Clone(cols), func(c api.Collection) bool { return c.ID != e.CalendarID })
		stays := len(w.initial.invitees) > 0 && slices.ContainsFunc(own, func(c api.Collection) bool { return c.Invites })
		if w.readOnly || stays {
			w.calendars = choice(own)
		} else {
			w.calendars = choice(writable)
		}
	}
	w.draft = w.initial
	w.draft.alarms = slices.Clone(w.initial.alarms)
	w.draft.invitees = slices.Clone(w.initial.invitees)
	w.zones = zoneChoices(zone, w.draft.timeZone)
	w.loaded = true
}

func (w *eventWindow) original() *eventDraft {
	if w.event == nil {
		return nil
	}
	return &w.initial
}

func (w *eventWindow) invited() bool {
	return w.event != nil && w.event.Organizer != nil && !w.event.Organizer.IsUser
}

func (w *eventWindow) dirty() bool {
	if w.readOnly || !w.loaded {
		return false
	}
	return w.typedInvitees() || w.draft.validate().any() || !emptyFields(changedFields(&w.initial, w.draft))
}

// takesInvitees reports whether the Add Invitee field shows.
func (w *eventWindow) takesInvitees() bool {
	return !w.readOnly && !w.invited() && w.calendarInvites()
}

// typedInvitees reports text in the Add Invitee field not yet committed.
func (w *eventWindow) typedInvitees() bool {
	return w.takesInvitees() && strings.TrimSpace(w.inviteeText) != ""
}

func (w *eventWindow) cannotSave() bool {
	if w.draft.validate().any() {
		return true
	}
	return sendsInvitees(w.original(), w.draft) && !w.calendarInvites()
}

func (w *eventWindow) calendarInvites() bool {
	for _, c := range w.calendars {
		if c.id == w.draft.calendarID {
			return c.invites
		}
	}
	return false
}

// closeNow closes the window, its changes saved or dropped: the app's
// Close asks closeRequested, which lets it go.
func (w *eventWindow) closeNow() {
	w.closed = true
	w.forget()
	if w.m.o.CloseWindow != nil {
		w.m.o.CloseWindow(w.key)
	}
}

// requestClose closes, or asks first when the draft has changes.
func (w *eventWindow) requestClose() {
	if w.busy {
		return
	}
	if w.dirty() {
		w.closing = true
		return
	}
	w.closeNow()
}

// closeRequested answers the system's close button, and the app's Close.
// A window closing for good, or whose maild the module has left, goes
// without a question: only that maild could take its changes.
func (w *eventWindow) closeRequested() bool {
	if w.closed || w.stale() {
		w.forget()
		return true
	}
	if w.busy || w.dirty() {
		w.requestClose()
		w.m.o.Wake()
		return false
	}
	w.forget()
	return true
}

// request starts a save or a delete.
func (w *eventWindow) request(del bool) {
	if w.busy || w.readOnly || (del && w.invited()) || w.stale() {
		return
	}
	if !del && w.typedInvitees() {
		w.commitInvitees()
		if w.cannotSave() { // what was typed is no address: show it
			w.closing = false
		}
	}
	if !del && w.cannotSave() {
		return
	}
	w.closing = false
	if !del && w.event != nil && !w.dirty() {
		w.closeNow()
		return
	}
	if w.event != nil && w.event.Recurring && w.event.RecurrenceID != "" {
		first := w.event.First != nil && *w.event.First
		w.scope = &scopeSheet{delete: del, first: first, choose: func(s api.EventScope) { w.perform(del, &s) }}
		return
	}
	w.perform(del, nil)
}

func (w *eventWindow) perform(del bool, scope *api.EventScope) {
	if (del && w.event == nil) || w.stale() {
		return
	}
	w.busy, w.saveFailure = true, ""
	event, draft, original := w.event, w.draft, w.original()
	fields := changedFields(original, draft)
	w.m.withNotify(&w.dialogs, func(ctx context.Context, c *api.Client, notify *bool) (string, error) {
		var rid *string
		if event != nil && scope != nil {
			rid = &event.RecurrenceID
		}
		var res *api.EventWrite
		var err error
		switch {
		case del:
			res, err = c.Calendar().Delete(ctx, &api.CalendarDeleteParams{ID: event.ID, RecurrenceID: rid, Scope: scope, Notify: notify})
		case event != nil:
			res, err = c.Calendar().Update(ctx, &api.CalendarUpdateParams{ID: event.ID, RecurrenceID: rid, Scope: scope, Fields: fields, Notify: notify})
		default:
			res, err = c.Calendar().Create(ctx, &api.CalendarCreateParams{CalendarID: &draft.calendarID, Fields: fields, Notify: notify})
		}
		if err != nil {
			return "", err
		}
		return res.RestoreID, nil
	}, func(o writeOutcome) {
		w.busy = false
		if w.stale() { // the window went with the maild it wrote to
			return
		}
		switch {
		case o.err != nil:
			w.saveFailure = o.err.Error()
		case !o.cancelled:
			w.m.rememberCalendar(o.restore)
			w.closeNow()
		}
	})
}

func (w *eventWindow) title() string {
	if w.loaded && w.draft.summary != "" {
		return w.draft.summary
	}
	return "New Event"
}

// view draws the window.
func (w *eventWindow) view(c *ui.Context) {
	frostui.Adapt(c)
	t := c.Theme()
	c.OnWindowShortcut(0, ui.KeyEscape, w.requestClose)
	ui.Column(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		ui.Row(c).Height(32).Shrink(0).Padding(0, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Center).DragWindow().Children(func() {
			frostui.Strong(c, w.title()).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			if frostui.IconButton(c, "window-close", "Close").Disabled(w.busy).Clicked() {
				w.requestClose()
			}
		})
		frostui.RowDivider(c)
		if !w.loaded {
			ui.Column(c).Padding(20).Children(func() {
				if w.failure != "" {
					frostui.Problem(c, w.failure)
				} else {
					frostui.Body(c, "Loading…")
				}
			})
			return
		}
		ui.Scroll(c).Grow(1).Fill().Children(func() {
			ui.Column(c).Padding(20).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Disabled(w.busy).DimDisabled(false).Children(func() { w.form(c) })
			if w.saveFailure != "" {
				ui.Row(c).Padding(0, 20, frostui.SpaceL, 20).Role(ui.RoleAlert).Children(func() { frostui.Problem(c, w.saveFailure) })
			}
		})
		frostui.RowDivider(c)
		w.footer(c)
	})
	drawScope(c, &w.scope)
	w.closeSheet(c)
	w.repeatSheetView(c)
	w.dialogs.View(c)
	if w.m.o.SetWindowTitle != nil {
		w.m.o.SetWindowTitle(w.key, w.title())
	}
}

func (w *eventWindow) footer(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(20).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		if w.event != nil && !w.readOnly && !w.invited() {
			if textButton(c, "Delete Event", flagColor(t, 1), 13).Disabled(w.busy).Clicked() {
				w.request(true)
			}
		}
		ui.Spacer(c)
		if w.readOnly {
			if ui.Button(c, "Close").Clicked() {
				w.requestClose()
			}
			return
		}
		if ui.Button(c, "Cancel").Disabled(w.busy).Clicked() {
			w.requestClose()
		}
		if ui.PrimaryButton(c, "Save").Disabled(w.busy || w.cannotSave()).Clicked() {
			w.request(false)
		}
	})
}

// closeSheet asks about unsaved changes.
func (w *eventWindow) closeSheet(c *ui.Context) {
	if !w.closing {
		return
	}
	save, drop := false, false
	cancelled := sheetFrame(c, "Save changes to \""+w.title()+"\"?", func() {
		ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.End).Children(func() {
			if ui.PrimaryButton(c, "Save").Disabled(w.busy || w.cannotSave()).AutoFocus().Clicked() {
				save = true
			}
			if ui.Button(c, "Don't Save").Clicked() {
				drop = true
			}
			if ui.Button(c, "Cancel").Clicked() {
				w.closing = false
			}
		})
	})
	switch {
	case save:
		w.request(false)
	case drop:
		w.closing = false
		w.closeNow()
	case cancelled:
		w.closing = false
	}
}

func editorRow(c *ui.Context, label string, fn func()) { labeled(c, label, 80, fn) }

func errorLine(c *ui.Context, text string) {
	if text != "" {
		ui.Text(c, text).FontSize(frostui.TextSize(c, 12)).TextColor(flagColor(c.Theme(), 1)).Wrap()
	}
}

// compose joins a draft's date and time for the date and time inputs.
func composeTime(date, wall string) time.Time {
	d, ok := parseDate(date)
	if !ok {
		return time.Time{}
	}
	return d.Add(time.Duration(minutesOf(wall)) * time.Minute)
}

func (w *eventWindow) form(c *ui.Context) {
	t := c.Theme()
	d := &w.draft
	errs := d.validate()
	disabled := w.readOnly || w.invited()
	ui.TextInput(c.Key("title"), &d.summary).Label("Title").Placeholder("New Event").FontSize(frostui.TextSize(c, 17)).FontWeight(600).Disabled(disabled).AutoFocus().OnSubmit(func() { w.request(false) })
	if w.readOnly {
		ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, "changes-prevent").FontSize(frostui.TextSize(c, 12)).TextColor(t.TextMuted)
			frostui.Muted(c, "Read-only")
		})
	}
	ui.TextInput(c.Key("location"), &d.location).Label("Location").Placeholder("Add Location").FontSize(frostui.TextSize(c, 13)).Disabled(disabled).OnSubmit(func() { w.request(false) })
	editorRow(c, "All-day", func() { ui.Checkbox(c, &d.allDay, "").Label("All-day").Disabled(disabled) })
	w.startT, w.endT = composeTime(d.startDate, d.startTime), composeTime(d.endDate, d.endTime)
	editorRow(c, "Starts", func() {
		ui.Row(c).Gap(frostui.SpaceS).Children(func() {
			if ui.DateInput(c.Key("start-date"), &w.startT).Label("Start date").Disabled(disabled).Invalid(errs.start != "").Changed() {
				*d = moveStart(*d, w.startT.Format(time.DateOnly), d.startTime)
			}
			if !d.allDay && ui.TimeInput(c.Key("start-time"), &w.startT).Label("Start time").Disabled(disabled).Invalid(errs.start != "").Changed() {
				*d = moveStart(*d, d.startDate, w.startT.Format("15:04"))
			}
		})
		errorLine(c, errs.start)
	})
	editorRow(c, "Ends", func() {
		ui.Row(c).Gap(frostui.SpaceS).Children(func() {
			if ui.DateInput(c.Key("end-date"), &w.endT).Label("End date").Disabled(disabled).Invalid(errs.end != "").Changed() {
				d.endDate = w.endT.Format(time.DateOnly)
			}
			if !d.allDay && ui.TimeInput(c.Key("end-time"), &w.endT).Label("End time").Disabled(disabled).Invalid(errs.end != "").Changed() {
				d.endTime = w.endT.Format("15:04")
			}
		})
		errorLine(c, errs.end)
	})
	if !d.allDay {
		editorRow(c, "Time zone", func() {
			ui.Select(c.Key("zone"), &d.timeZone, w.zones).Label("Time zone").Disabled(disabled)
			errorLine(c, errs.timeZone)
		})
	}
	w.repeatRow(c, disabled)
	if d.rrule != "" {
		w.endRepeatRow(c, disabled)
	}
	w.calendarRow(c, disabled)
	w.inviteesRow(c, disabled, errs.invitees)
	w.alertsRow(c)
	editorRow(c, "Show as", func() {
		choice := "Busy"
		if d.transparent {
			choice = "Free"
		}
		if ui.Select(c.Key("show-as"), &choice, []string{"Busy", "Free"}).Label("Show as").Disabled(disabled).Changed() {
			d.transparent = choice == "Free"
		}
	})
	ui.TextArea(c.Key("notes"), &d.description).Label("Notes").FontSize(frostui.TextSize(c, 13)).MinHeight(120).Disabled(disabled)
}

func (w *eventWindow) repeatRow(c *ui.Context, disabled bool) {
	d := &w.draft
	preset := presetOf(d.rrule, d.startDate)
	options := make([]string, 0, 9)
	current := ""
	for _, p := range repeatLabels {
		options = append(options, p.label)
		if p.preset == preset {
			current = p.label
		}
	}
	if preset == presetCustom {
		current = describeRecurrence("RRULE:" + d.rrule)
		options = append(options, current)
	}
	options = append(options, "Custom…")
	loc := d.zoneLoc()
	editorRow(c, "Repeat", func() {
		if ui.Select(c.Key("repeat"), &current, options).Label("Repeat").Disabled(disabled).Changed() {
			if current == "Custom…" {
				r := defaultRepeat(d.startDate)
				if have := repeatOf(d.rrule, d.startDate); have != nil && d.rrule != "" {
					r = *have
				}
				w.repeat = &repeatSheet{r: r, interval: strconv.Itoa(r.interval)}
				return
			}
			for _, p := range repeatLabels {
				if p.label == current {
					if p.preset == presetNever {
						d.rrule = ""
					} else {
						d.rrule = withEnd(presetRule(p.preset), endOf(d.rrule, loc), d.allDay, loc)
					}
				}
			}
		}
	})
}

func (w *eventWindow) endRepeatRow(c *ui.Context, disabled bool) {
	d := &w.draft
	loc := d.zoneLoc()
	end := endOf(d.rrule, loc)
	labels := map[string]string{"never": "Never", "date": "On Date", "count": "After"}
	choice := labels[end.kind]
	editorRow(c, "End repeat", func() {
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			if ui.Select(c.Key("end-repeat"), &choice, []string{"Never", "On Date", "After"}).Label("End repeat").Disabled(disabled).Changed() {
				switch choice {
				case "On Date":
					end = repeatEnd{kind: "date", date: d.startDate}
				case "After":
					end = repeatEnd{kind: "count", count: 10}
				default:
					end = repeatEnd{kind: "never"}
				}
				d.rrule = withEnd(d.rrule, end, d.allDay, loc)
			}
			switch end.kind {
			case "date":
				w.untilT = dateOf(end.date)
				if ui.DateInput(c.Key("until"), &w.untilT).Label("End repeat date").Disabled(disabled).Changed() {
					d.rrule = withEnd(d.rrule, repeatEnd{kind: "date", date: w.untilT.Format(time.DateOnly)}, d.allDay, loc)
				}
			case "count":
				n := float64(end.count)
				if ui.NumberInput(c.Key("count"), &n, 1, 999, 1).Label("Repeat count").Width(80).Disabled(disabled).Changed() {
					d.rrule = withEnd(d.rrule, repeatEnd{kind: "count", count: int(n)}, d.allDay, loc)
				}
				frostui.Body(c, "times")
			}
		})
	})
}

func (w *eventWindow) calendarRow(c *ui.Context, disabled bool) {
	t := c.Theme()
	d := &w.draft
	name := func(id int64) (string, ui.Color) {
		for _, ch := range w.calendars {
			if ch.id == id {
				return ch.name + " · " + ch.account, calColor(t, ch.color)
			}
		}
		return "", t.Accent
	}
	editorRow(c, "Calendar", func() {
		sel := ui.SelectBase(c.Key("calendar"), &d.calendarID)
		sel.Trigger.Gap(6).Padding(6, 10).Radius(frostui.RadiusChip).Border(1, t.Border).AlignItems(ui.Center).Label("Calendar").Disabled(disabled).Children(func() {
			label, color := name(d.calendarID)
			uikit.Dot(c, 8, color)
			frostui.Body(c, label).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		})
		sel.Popup(func(panel ui.Element) {
			panel.Padding(4).Radius(frostui.RadiusCard).Background(t.Background).Border(1, t.Border)
			for _, ch := range w.calendars {
				item := sel.Item(ch.id).Padding(6, 10).Radius(frostui.RadiusChip).Gap(6).AlignItems(ui.Center).Label(ch.name)
				if item.Highlighted() {
					item.Background(t.Accent).TextColor(t.AccentText)
				}
				item.Children(func() {
					uikit.Dot(c, 8, calColor(t, ch.color))
					frostui.Body(c, ch.name+" · "+ch.account).SingleLine()
				})
			}
		})
	})
}

func (w *eventWindow) alertsRow(c *ui.Context) {
	t := c.Theme()
	d := &w.draft
	presets := timedAlerts
	if d.allDay {
		presets = allDayAlerts
	}
	readOnly := w.readOnly
	editorRow(c, "Alerts", func() {
		removed := -1
		for i, alarm := range d.alarms {
			options := []string{"None"}
			current := alarmText(alarm)
			known := false
			for _, p := range presets {
				options = append(options, p.label)
				if p.minutes == alarm {
					current, known = p.label, true
				}
			}
			if !known {
				options = append(options, current)
			}
			if ui.Select(c.Key(i), &current, options).Label(fmt.Sprintf("Alert %d", i+1)).Disabled(readOnly).Changed() {
				if current == "None" {
					removed = i
				}
				for _, p := range presets {
					if p.label == current {
						d.alarms[i] = p.minutes
					}
				}
			}
		}
		if removed >= 0 {
			d.alarms = slices.Delete(d.alarms, removed, removed+1)
		}
		next := slices.IndexFunc(presets, func(p alertChoice) bool { return !slices.Contains(d.alarms, p.minutes) })
		if addButton(c.Key("add"), "add alert", t.Accent).Disabled(readOnly || next < 0).Clicked() {
			d.alarms = append(d.alarms, presets[next].minutes)
		}
	})
}

func (w *eventWindow) inviteesRow(c *ui.Context, disabled bool, problem string) {
	t := c.Theme()
	d := &w.draft
	invites := w.calendarInvites()
	frozen := disabled || !invites
	editorRow(c, "Invitees", func() {
		removed := -1
		for i := range d.invitees {
			inv := &d.invitees[i]
			valid := validAddress(inv.email)
			name := inv.name
			if name == "" {
				name = inv.email
			}
			tip := name + " <" + inv.email + ">"
			color := t.Text
			if !valid {
				color, tip = flagColor(t, 1), inv.email+" is not a valid address"
			}
			ui.Row(c.Key(inv.email)).Height(28).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Body(c, name).TextColor(color).Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(tip)
				answerIcon(c, inv.answer)
				if inv.role == api.AttendeeRoleChair {
					ui.Text(c, "Chair").FontSize(frostui.TextSize(c, 12)).TextColor(t.TextMuted)
				}
				if inv.role == api.AttendeeRoleNone {
					ui.Text(c, "For information").FontSize(frostui.TextSize(c, 12)).TextColor(t.TextMuted)
				}
				optional := t.TextMuted
				if inv.role == api.AttendeeRoleOptional {
					optional = t.Accent
				}
				if textButton(c.Key("optional"), "Optional", optional, 12).Checked(inv.role == api.AttendeeRoleOptional).Disabled(frozen).Clicked() {
					if inv.role == api.AttendeeRoleOptional {
						inv.role = api.AttendeeRoleRequired
					} else {
						inv.role = api.AttendeeRoleOptional
					}
				}
				if frostui.IconButton(c.Key("remove"), "window-close", "Remove "+name).Disabled(frozen).Clicked() {
					removed = i
				}
			})
		}
		if removed >= 0 {
			d.invitees = slices.Delete(d.invitees, removed, removed+1)
		}
		errorLine(c, problem)
		if !invites {
			ui.Text(c, "Invitations aren't available in this calendar.").FontSize(frostui.TextSize(c, 12)).TextColor(tertiary(t)).Wrap()
			return
		}
		if disabled {
			return
		}
		in := ui.TextInput(c.Key("add-invitee"), &w.inviteeText).Label("Add Invitee").FontSize(frostui.TextSize(c, 13))
		if in.Changed() {
			if strings.ContainsAny(w.inviteeText, ",;\n") {
				w.commitInvitees()
			} else {
				w.askSuggestions()
			}
		}
		if in.Submitted() {
			w.commitInvitees()
		}
		for _, s := range w.suggestions {
			label := s.Address
			if s.Name != "" {
				label = s.Name + " <" + s.Address + ">"
			}
			if addButton(c.Key(s.Address), label, t.Accent).Clicked() {
				w.addInvitee(s.Address, s.Name)
				w.inviteeText, w.suggestions = "", nil
			}
		}
	})
}

// commitInvitees turns the typed text into invitees.
func (w *eventWindow) commitInvitees() {
	for piece := range strings.FieldsFuncSeq(w.inviteeText, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		name, addr := splitAddress(strings.TrimSpace(piece))
		if addr != "" {
			w.addInvitee(addr, name)
		}
	}
	w.inviteeText, w.suggestions = "", nil
}

// splitAddress reads "Name <address>" or a bare address.
func splitAddress(piece string) (name, address string) {
	if open := strings.LastIndex(piece, "<"); open >= 0 && strings.HasSuffix(piece, ">") {
		return strings.Trim(strings.TrimSpace(piece[:open]), `"`), strings.TrimSpace(piece[open+1 : len(piece)-1])
	}
	return "", piece
}

func (w *eventWindow) addInvitee(address, name string) {
	if slices.ContainsFunc(w.draft.invitees, func(i draftInvitee) bool { return strings.EqualFold(i.email, address) }) {
		return
	}
	w.draft.invitees = append(w.draft.invitees, draftInvitee{email: address, name: name})
}

func (w *eventWindow) askSuggestions() {
	m := w.m
	c := m.client
	prefix := strings.TrimSpace(w.inviteeText)
	w.suggestToken++
	token := w.suggestToken
	if c == nil || prefix == "" {
		w.suggestions = nil
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		found, err := c.Address().Suggest(ctx, &api.AddressSuggestParams{Prefix: prefix, Limit: ptr(int64(8))})
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("suggest addresses", "err", err)
		}
		m.o.Post(func() {
			if token != w.suggestToken {
				return
			}
			w.suggestions = slices.DeleteFunc(found, func(a api.Address) bool {
				return slices.ContainsFunc(w.draft.invitees, func(i draftInvitee) bool { return strings.EqualFold(i.email, a.Address) })
			})
		})
	}()
}

// repeatSheetView draws Custom Repeat.
func (w *eventWindow) repeatSheetView(c *ui.Context) {
	s := w.repeat
	if s == nil {
		return
	}
	t := c.Theme()
	d := &w.draft
	freqs := []string{"Daily", "Weekly", "Monthly", "Yearly"}
	values := []string{"DAILY", "WEEKLY", "MONTHLY", "YEARLY"}
	units := []string{"days", "weeks", "months", "years"}
	at := max(slices.Index(values, s.r.frequency), 0)
	done, cancel := false, false
	dismissed := sheetFrame(c, "Custom Repeat", func() {
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			frostui.Body(c, "Frequency")
			choice := freqs[at]
			if ui.Select(c.Key("frequency"), &choice, freqs).Label("Frequency").Changed() {
				s.r.frequency = values[slices.Index(freqs, choice)]
				if s.r.frequency == "WEEKLY" && len(s.r.weekdays) == 0 {
					s.r.weekdays = defaultRepeat(d.startDate).weekdays
				}
			}
		})
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			frostui.Body(c, "Every")
			ui.TextInput(c.Key("every"), &s.interval).Label("Every").Width(60).FontSize(frostui.TextSize(c, 13))
			frostui.Body(c, units[at])
		})
		if s.r.frequency == "WEEKLY" {
			ui.Row(c).Gap(frostui.SpaceXS).Children(func() {
				ordered := weekOrder(w.m.clock.weekStart)
				for _, day := range ordered {
					on := slices.Contains(s.r.weekdays, day)
					b := ui.ButtonBase(c.Key(day)).Size(32, 28).Radius(t.Radius).Center().Border(1, t.Border).Label(dayNames[day]).Checked(on).Cursor(ui.CursorPointer)
					color := t.Text
					if on {
						b.Background(t.Accent)
						color = t.AccentText
					}
					if b.Clicked() {
						if on {
							s.r.weekdays = slices.DeleteFunc(s.r.weekdays, func(x string) bool { return x == day })
						} else {
							s.r.weekdays = append(s.r.weekdays, day)
						}
					}
					b.Children(func() { ui.Text(c, dayNames[day][:1]).FontSize(frostui.TextSize(c, 12)).TextColor(color) })
				}
			})
		}
		if s.r.frequency == "MONTHLY" {
			ui.RadioGroup(c, func() {
				ui.Radio(c.Key("day"), &s.r.byWeekday, false, "on day "+strconv.Itoa(dateOf(d.startDate).Day()))
				ui.Radio(c.Key("weekday"), &s.r.byWeekday, true, monthlyByDayLabel(s.r.monthlyByDay(d.startDate), d.startDate))
			})
		}
		ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				cancel = true
			}
			_, valid := s.every()
			if ui.PrimaryButton(c, "OK").Disabled(!valid).Clicked() {
				done = true
			}
		})
	})
	switch {
	case done:
		s.r.interval, _ = s.every()
		loc := d.zoneLoc()
		d.rrule = withEnd(ruleOfRepeat(s.r, d.startDate), endOf(d.rrule, loc), d.allDay, loc)
		w.repeat = nil
	case cancel || dismissed:
		w.repeat = nil
	}
}

// weekOrder is the days from the week's first day.
func weekOrder(weekStart int) []string {
	out := make([]string, 7)
	for i := range out {
		out[i] = weekdayNames[(weekStart+6+i)%7]
	}
	return out
}

// loadZone reads a zone, nil when unknown.
func loadZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// zoneChoices lists the app's zone first, then the event's own, then the
// zones the system knows, by name.
func zoneChoices(app, own string) []string {
	out := []string{}
	add := func(z string) {
		if z != "" && !slices.Contains(out, z) {
			out = append(out, z)
		}
	}
	add(app)
	add(own)
	for _, z := range systemZones() {
		add(z)
	}
	return out
}

// systemZones walks the system's zone files; none on a system without
// them.
func systemZones() []string {
	const root = "/usr/share/zoneinfo"
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // A folder that cannot be read is left out.
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == "posix" || rel == "right" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.Contains(rel, "/") && rel[0] >= 'A' && rel[0] <= 'Z' && !strings.HasPrefix(rel, "Etc/") {
			out = append(out, rel)
		}
		return nil
	})
	if _, err := os.Stat(root); err != nil {
		out = append(out, "UTC")
	}
	slices.Sort(out)
	return out
}
