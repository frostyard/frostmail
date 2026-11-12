package pim

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// occKey names one occurrence.
type occKey struct {
	eventID      int64
	recurrenceID string
}

func keyOf(o *api.Occurrence) occKey { return occKey{o.EventID, o.RecurrenceID} }

// miniMonthState is what a small month remembers.
type miniMonthState struct{ shown, selection string }

// syncSelection follows a newly selected day without resetting a month
// the user browsed with the small calendar's arrows.
func (st *miniMonthState) syncSelection(selected string) {
	if st.selection != selected {
		st.selection, st.shown = selected, selected
	}
}

// calendarState is the Calendar module's state.
type calendarState struct {
	view        viewKind
	date        string
	selected    *occKey
	focus       string // the pane holding the keyboard
	focusAsk    string // the pane to focus next frame
	collections fetch[[]api.Collection]
	occ         fetch[[]api.Occurrence]
	mini        fetch[[]api.Occurrence]
	event       fetch[*api.CalendarEvent]
	series      fetch[[]string] // the UIDs of the events that may be the selection's override
	month       miniMonthState
	scroll      ui.ScrollState
	timeGridOn  bool
	press       *gridPress
	writable    map[int64]bool
	grab        *barGrab
	goneCheck   [3]int // the answers the selection was last checked against
	scope       *scopeSheet
	undo        []string
	cellHeight  float32

	layoutKey string
	blocks    map[string][]timedBlock
	bars      []allDayBar
	cells     map[string][]*api.Occurrence
	busyKey   string
	busy      map[string]bool
}

func (m *Module) setUpCalendar() {
	m.cal.view = viewWeek
	m.cal.focus = paneCalView
}

func (m *Module) calendarChanged() {
	m.cal.occ.dirty, m.cal.mini.dirty, m.cal.event.dirty, m.cal.series.dirty = true, true, true, true
	m.todo.mini.dirty, m.todo.upcoming.dirty = true, true
	m.remindersChanged()
}

// loadCalendars asks for the calendars.
func (m *Module) loadCalendars() {
	want(m, &m.cal.collections, "calendars", func(ctx context.Context, c *api.Client) ([]api.Collection, error) {
		kind := api.CollectionKindCalendar
		return c.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
	})
}

// loadRange asks for a range of occurrences, in the app's zone. maild may
// be on another machine, so a zone with no name to send is never left to
// maild's: the call asks in UTC, a day wider, and keeps what falls in the
// range here.
func (m *Module) loadRange(f *fetch[[]api.Occurrence], from, to, extra string) {
	zone, loc := m.zone, m.clock.loc
	want(m, f, rangeKey(from, to, zone, extra), func(ctx context.Context, c *api.Client) ([]api.Occurrence, error) {
		if zone != localZone {
			return c.Calendar().Range(ctx, &api.CalendarRangeParams{From: from, To: to, TimeZone: &zone})
		}
		utc := "UTC"
		wide, err := c.Calendar().Range(ctx, &api.CalendarRangeParams{From: addDays(from, -1), To: addDays(to, 1), TimeZone: &utc})
		if err != nil {
			return nil, err
		}
		return inRange(wide, from, to, loc), nil
	})
}

func rangeKey(from, to, zone, extra string) string { return from + "|" + to + "|" + zone + "|" + extra }

// inRange keeps the occurrences in a range of dates in a zone, with
// all-day ones at that zone's midnights, as a call naming the zone answers.
func inRange(occs []api.Occurrence, from, to string, loc *time.Location) []api.Occurrence {
	start, end := dayStart(from, loc), dayStart(to, loc)
	out := make([]api.Occurrence, 0, len(occs))
	for _, o := range occs {
		if o.AllDay {
			if o.StartDate >= to || o.EndDate <= from {
				continue
			}
			o.Start, o.End = dayStart(o.StartDate, loc), dayStart(o.EndDate, loc)
		} else if !overlaps(&o, start, end) {
			continue
		}
		out = append(out, o)
	}
	slices.SortStableFunc(out, func(a, b api.Occurrence) int { return a.Start.Compare(b.Start) })
	return out
}

// syncCalendar asks for what the module shows and lays it out when it
// changed.
func (m *Module) syncCalendar() {
	cal := &m.cal
	m.loadCalendars()
	cal.writable = m.writableCalendars()
	from, to := viewRange(cal.view, cal.date, m.clock.weekStart)
	m.loadRange(&cal.occ, from, to, "")
	cal.month.syncSelection(cal.date)
	days := monthGrid(cal.month.shown, m.clock.weekStart)
	m.loadRange(&cal.mini, days[0], addDays(days[41], 1), "")
	m.syncEvent()
	m.endRemovedOccurrence(from, to)
	viewDaysNow := viewDays(cal.view, cal.date, m.clock.weekStart)
	key := fmt.Sprint(cal.occ.version, "|", string(cal.view), "|", viewDaysNow[0], "|", m.zone)
	if key != cal.layoutKey {
		cal.layoutKey = key
		occs := cal.occ.value
		if cal.view == viewMonth {
			cal.cells = make(map[string][]*api.Occurrence, 42)
			for _, d := range viewDaysNow {
				cal.cells[d] = monthItems(occs, d, m.clock.loc)
			}
		} else {
			cal.blocks = timedLayout(occs, viewDaysNow, m.clock.loc)
			cal.bars = allDayLanes(occs, viewDaysNow)
		}
	}
	busyKey := fmt.Sprint(cal.mini.version, "|", days[0], "|", m.zone)
	if busyKey != cal.busyKey {
		cal.busyKey = busyKey
		cal.busy = busyDates(cal.mini.value, days, m.clock.loc)
	}
}

// syncEvent asks for the selected event; when it is gone the selection
// ends.
func (m *Module) syncEvent() {
	cal := &m.cal
	if cal.selected == nil {
		return
	}
	sel := *cal.selected
	want(m, &cal.event, fmt.Sprint(sel.eventID, "|", sel.recurrenceID), func(ctx context.Context, c *api.Client) (*api.CalendarEvent, error) {
		p := &api.CalendarEventParams{ID: sel.eventID}
		if sel.recurrenceID != "" {
			p.RecurrenceID = &sel.recurrenceID
		}
		return c.Calendar().Event(ctx, p)
	})
	if cal.event.loaded && cal.event.err != nil && cal.event.key == fmt.Sprint(m.conn, "|", fmt.Sprint(sel.eventID, "|", sel.recurrenceID)) {
		cal.selected, cal.event.value, cal.event.err = nil, nil, nil
	}
}

// endRemovedOccurrence ends the selection of an occurrence taken out of
// its series. maild still answers for it from the series, so only the
// shown range can tell: once the range and the event have both answered,
// an occurrence dated inside the range that the range lacks is gone.
//
// Another series can have an occurrence at the same time, with the same
// recurrence ID, so the range's occurrence must be the series' own: the
// series' event ID, or an override of the series. An override has
// another event ID and the same recurrence ID, and stays; maild names
// its series, the event's UID, which another series' event does not
// share.
func (m *Module) endRemovedOccurrence(from, to string) {
	cal := &m.cal
	sel := cal.selected
	if sel == nil || sel.recurrenceID == "" || m.client == nil {
		return
	}
	occ, event := &cal.occ, &cal.event
	if occ.key != fmt.Sprint(m.conn, "|", rangeKey(from, to, m.zone, "")) || !occ.loaded || occ.err != nil || occ.pending || occ.dirty || event.pending || event.dirty {
		return
	}
	// The series' answer counts too: when it comes, the check runs again.
	answers := [3]int{occ.version, event.version, cal.series.version}
	if answers == cal.goneCheck {
		return
	}
	cal.goneCheck = answers
	e := m.shownEvent()
	if e == nil || !m.eventInRange(e, from, to) {
		return
	}
	if col := m.collection(e.CalendarID); col != nil && !col.Enabled {
		return
	}
	var others []int64
	for _, o := range occ.value {
		if o.RecurrenceID != sel.recurrenceID || o.CalendarID != e.CalendarID {
			continue
		}
		if o.EventID == sel.eventID {
			return
		}
		others = append(others, o.EventID)
	}
	if len(others) > 0 {
		key := fmt.Sprint(others)
		want(m, &cal.series, key, func(ctx context.Context, c *api.Client) ([]string, error) {
			uids := make([]string, 0, len(others))
			for _, id := range others {
				ev, err := c.Calendar().Event(ctx, &api.CalendarEventParams{ID: id})
				if err != nil {
					return nil, err
				}
				uids = append(uids, ev.UID)
			}
			return uids, nil
		})
		s := &cal.series
		// Until maild has named them, or when it could not, the selection stays.
		if s.key != fmt.Sprint(m.conn, "|", key) || !s.loaded || s.pending || s.err != nil || slices.Contains(s.value, e.UID) {
			return
		}
	}
	cal.selected, cal.event.value = nil, nil
}

func (m *Module) eventInRange(e *api.CalendarEvent, from, to string) bool {
	if e.AllDay {
		return e.StartDate < to && e.EndDate > from
	}
	return overlaps(&api.Occurrence{Start: e.Start, End: e.End}, dayStart(from, m.clock.loc), dayStart(to, m.clock.loc))
}

// shownEvent is the selected event once loaded.
func (m *Module) shownEvent() *api.CalendarEvent {
	e := m.cal.event.value
	if m.cal.selected == nil || e == nil || e.ID != m.cal.selected.eventID || e.RecurrenceID != m.cal.selected.recurrenceID {
		return nil
	}
	return e
}

func (m *Module) writableCalendars() map[int64]bool {
	out := map[int64]bool{}
	for _, cal := range m.cal.collections.value {
		if !cal.ReadOnly && !m.accountReadOnly(cal.AccountID) {
			out[cal.ID] = true
		}
	}
	return out
}

func (m *Module) collection(id int64) *api.Collection {
	for i := range m.cal.collections.value {
		if m.cal.collections.value[i].ID == id {
			return &m.cal.collections.value[i]
		}
	}
	return nil
}

func (m *Module) colorOf(t *ui.Theme, calendar int64) ui.Color {
	if col := m.collection(calendar); col != nil {
		return calColor(t, col.Color)
	}
	return t.Accent
}

// selectOccurrence selects an occurrence and its date.
func (m *Module) selectOccurrence(o *api.Occurrence) {
	k := keyOf(o)
	m.cal.selected = &k
	m.cal.date = m.occDate(o)
}

func (m *Module) occDate(o *api.Occurrence) string {
	if o.AllDay {
		return o.StartDate
	}
	d, _ := zoned(o.Start, m.clock.loc)
	return d
}

// ShowOccurrence selects an occurrence from elsewhere, keeping the view.
func (m *Module) ShowOccurrence(o api.Occurrence) {
	m.selectOccurrence(&o)
	m.show(commands.Calendar)
}

// setDate selects a date; the view follows it.
func (m *Module) setDate(date string) { m.cal.date = date }

func (m *Module) showDay(date string) {
	m.cal.date = date
	m.cal.view = viewDay
}

func (m *Module) page(direction int) { m.cal.date = stepDate(m.cal.view, m.cal.date, direction) }

func (m *Module) calendarHandlers() map[commands.Command]func() {
	view := func(f func()) func() {
		return func() {
			if m.cal.focus == paneCalView {
				f()
			}
		}
	}
	cal := &m.cal
	move := func(step int) func() {
		return view(func() {
			if cal.view == viewMonth {
				cal.date = addDays(cal.date, step*7)
			} else {
				cal.scroll.Y += float32(step) * hourHeight
			}
		})
	}
	return map[commands.Command]func(){
		commands.DayView:        func() { cal.view = viewDay },
		commands.WeekView:       func() { cal.view = viewWeek },
		commands.MonthView:      func() { cal.view = viewMonth },
		commands.Today:          func() { cal.date = m.today() },
		commands.PreviousPeriod: func() { m.page(-1) },
		commands.NextPeriod:     func() { m.page(1) },
		commands.Left:           view(func() { cal.date = addDays(cal.date, -1) }),
		commands.Right:          view(func() { cal.date = addDays(cal.date, 1) }),
		commands.Previous:       move(-1),
		commands.Next:           move(1),
		commands.Open:           view(m.openSelected),
		commands.Escape:         m.calendarEscape,
		commands.Compose:        m.newEventDefault,
		commands.Delete:         m.deleteSelected,
		commands.Undo:           m.undoCalendar,
		commands.NextPane:       func() { m.cycleCalendar(1) },
		commands.PreviousPane:   func() { m.cycleCalendar(-1) },
	}
}

// calendarEscape gives up a drag first, then the selection.
func (m *Module) calendarEscape() {
	if p := m.cal.press; p != nil && p.dragging && !p.canceled {
		p.canceled, p.preview = true, nil
		return
	}
	m.cal.selected = nil
}

func (m *Module) openSelected() {
	if m.cal.selected == nil {
		m.showDay(m.cal.date)
		return
	}
	m.editSelected()
}

func (m *Module) cycleCalendar(step int) {
	panes := []string{paneSidebar, paneCalView, paneCalEvent}
	i := max(slices.Index(panes, m.cal.focus), 0)
	m.cal.focusAsk = panes[(i+step+len(panes))%len(panes)]
	m.cal.focus = m.cal.focusAsk
}

func (m *Module) calendarPanes(c *ui.Context) []frostui.Pane {
	m.syncCalendar()
	return []frostui.Pane{
		{ID: paneSidebar, Label: "Calendars", Width: sidebarWidth, Min: 160, Max: 320, Collapsible: true, Content: func() { m.calendarSidebar(c) }},
		{ID: paneCalView, Label: "Calendar view", Min: 360, Bar: func() { m.calendarBar(c) }, Content: func() { m.calendarView(c) }},
		{ID: paneCalEvent, Label: "Event details", Width: detailWidth, Min: detailWidth, Max: detailWidth, Content: func() { m.eventPane(c) }},
	}
}

// calendarBar is the top bar's segment over the view.
func (m *Module) calendarBar(c *ui.Context) {
	t := c.Theme()
	cal := &m.cal
	if frostui.IconButton(c, "list-add", "New Event (Ctrl+N)").Disabled(len(cal.writable) == 0).Clicked() {
		m.newEventDefault()
	}
	if ui.Button(c, "Today").Clicked() {
		cal.date = m.today()
	}
	period := map[viewKind]string{viewDay: "Day", viewWeek: "Week", viewMonth: "Month"}[cal.view]
	if frostui.IconButton(c, "go-previous", "Previous "+period).Clicked() {
		m.page(-1)
	}
	if frostui.IconButton(c, "go-next", "Next "+period).Clicked() {
		m.page(1)
	}
	frostui.Heading(c, viewTitle(cal.view, cal.date, m.clock.weekStart)).TextColor(t.Text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
	choice := slices.Index([]viewKind{viewDay, viewWeek, viewMonth}, cal.view)
	if ui.Segmented(c, &choice, "Day", "Week", "Month").Label("View").Changed() {
		cal.view = []viewKind{viewDay, viewWeek, viewMonth}[choice]
	}
}

// miniMonth draws the small month.
func (m *Module) miniMonth(c *ui.Context, st *miniMonthState, selected string, busy map[string]bool, onSelect func(string)) {
	t := c.Theme()
	st.syncSelection(selected)
	days := monthGrid(st.shown, m.clock.weekStart)
	title := viewTitle(viewMonth, st.shown, m.clock.weekStart)
	today := m.today()
	ui.Column(c).Padding(frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			if frostui.IconButton(c, "go-previous", "Previous Month").Clicked() {
				st.shown = stepDate(viewMonth, st.shown, -1)
			}
			frostui.Strong(c, title).SingleLine()
			if frostui.IconButton(c, "go-next", "Next Month").Clicked() {
				st.shown = stepDate(viewMonth, st.shown, 1)
			}
		})
		ui.Column(c).Role(ui.RoleGrid).Label(title).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Children(func() {
				for _, d := range days[:7] {
					ui.Text(c.Key(d), dateOf(d).Format("Mon")[:1]).FontSize(frostui.TextSize(c, 11)).TextColor(tertiary(t)).Grow(1).Basis(0).TextAlign(ui.Center)
				}
			})
			for week := range 6 {
				ui.Row(c.Key(week)).Children(func() {
					for _, date := range days[week*7 : week*7+7] {
						m.miniDay(c.Key(date), date, selected, today, st.shown, busy, onSelect)
					}
				})
			}
		})
	})
}

func (m *Module) miniDay(c *ui.Context, date, selected, today, shown string, busy map[string]bool, onSelect func(string)) {
	t := c.Theme()
	active, current := date == selected, date == today
	color := t.Text
	weight := 400
	var fill ui.Color
	switch {
	case active && current:
		fill, color = t.Accent, t.AccentText
	case active:
		fill = t.SurfacePressed
	case current:
		color, weight = t.Accent, 600
	case date[:7] != shown[:7]:
		color = tertiary(t)
	}
	ui.Box(c).Grow(1).Basis(0).Center().Children(func() {
		b := ui.ButtonBase(c).Size(24, 24).Radius(t.Radius).Center().Cursor(ui.CursorPointer).Label(longDate(date)).Checked(active)
		if current {
			b.Current(ui.CurrentDate)
		}
		if fill.A != 0 {
			b.Background(fill)
		}
		if b.Clicked() {
			onSelect(date)
		}
		if busy[date] {
			dot := tertiary(t)
			b.DrawOver(func(p *ui.Painter, r ui.Rect) {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 2, Y: r.Y + r.H - 4, W: 4, H: 4}, dot, 2)
			})
		}
		b.Children(func() {
			ui.Text(c, strconv.Itoa(dateOf(date).Day())).FontSize(frostui.TextSize(c, 12)).FontWeight(weight).TextColor(color).SingleLine()
		})
	})
}

// calendarSidebar draws the small month and the calendars.
func (m *Module) calendarSidebar(c *ui.Context) {
	cal := &m.cal
	t := c.Theme()
	s := ui.Scroll(c).Grow(1).Fill().Focusable().TabStop(false).Background(frostui.Chrome(t)).Label("Calendars")
	if cal.focusAsk == paneSidebar {
		cal.focusAsk = ""
		s.Focus()
	}
	if s.FocusWithin() {
		cal.focus = paneSidebar
	}
	s.Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			m.miniMonth(c, &cal.month, cal.date, cal.busy, m.setDate)
			for _, a := range m.accounts.value {
				var mine []api.Collection
				for _, col := range cal.collections.value {
					if col.AccountID == a.ID {
						mine = append(mine, col)
					}
				}
				if len(mine) == 0 {
					continue
				}
				ui.Column(c.Key(a.ID)).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label(a.Email).Children(func() {
					frostui.Label(c, a.Email).Height(26).Padding(6, 0, 0, frostui.SpaceM).SingleLine()
					for _, col := range mine {
						m.calendarRow(c.Key(col.ID), col, a.ReadOnly)
					}
				})
			}
		})
	})
}

func (m *Module) calendarRow(c *ui.Context, col api.Collection, accountReadOnly bool) {
	t := c.Theme()
	readOnly := col.ReadOnly || accountReadOnly
	color := calColor(t, col.Color)
	row := ui.ButtonBase(c).Row().Height(28).Padding(0, frostui.SpaceS, 0, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Center).
		Role(ui.RoleCheckBox).Checked(col.Enabled).Label(col.Name).Cursor(ui.CursorPointer)
	if row.Clicked() {
		id, on := col.ID, !col.Enabled
		m.run("change the calendar", func(ctx context.Context, c *api.Client) error {
			_, err := c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: id, Enabled: &on})
			return err
		}, nil)
	}
	row.ContextMenu(func(menu *ui.Menu) {
		if menu.Item("Use as Default Calendar").Checked(col.IsDefault).Disabled(col.IsDefault || readOnly).Chosen() {
			id := col.ID
			m.run("change the default calendar", func(ctx context.Context, c *api.Client) error {
				_, err := c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: id, IsDefault: ptr(true)})
				return err
			}, nil)
		}
	})
	row.Children(func() {
		box := ui.Box(c).Size(14, 14).Radius(frostui.RadiusChip).Border(1.5, color).Shrink(0)
		if col.Enabled {
			box.Background(color)
		}
		frostui.Body(c, col.Name).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if readOnly {
			frostui.Icon(c, "changes-prevent").FontSize(frostui.TextSize(c, 12)).TextColor(t.TextMuted).Label("Read-only")
		}
	})
}

// eventPane draws the selected event's details.
func (m *Module) eventPane(c *ui.Context) {
	t := c.Theme()
	cal := &m.cal
	e := m.shownEvent()
	pane := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Focusable().TabStop(false).Background(t.Background).Label("Event details")
	if cal.focusAsk == paneCalEvent {
		cal.focusAsk = ""
		pane.Focus()
	}
	if pane.FocusWithin() {
		cal.focus = paneCalEvent
	}
	pane.Children(func() {
		if e == nil {
			centered(c, "No Event Selected", 13)
			return
		}
		ui.Scroll(c).Grow(1).Fill().Children(func() {
			ui.Column(c).Padding(20).Gap(frostui.SpaceL).AlignItems(ui.Stretch).Children(func() { m.eventDetails(c, e) })
		})
	})
}

// eventRights says what the user can do to an event.
func (m *Module) eventRights(e *api.CalendarEvent) (writable, invited bool) {
	return !e.ReadOnly && m.writableCalendars()[e.CalendarID], e.Organizer != nil && !e.Organizer.IsUser
}

func (m *Module) eventDetails(c *ui.Context, e *api.CalendarEvent) {
	t := c.Theme()
	writable, invited := m.eventRights(e)
	k := m.clock
	ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			title := e.Summary
			if title == "" {
				title = "No Title"
			}
			ui.Text(c, title).FontSize(frostui.TextSize(c, 17)).FontWeight(600).Wrap().Role(ui.RoleHeading).Selectable()
			if e.Location != "" {
				frostui.Body(c, e.Location).TextColor(t.TextMuted).Wrap().Selectable()
			}
			if e.Status == api.EventStatusCancelled {
				ui.Text(c, "Cancelled").FontSize(frostui.TextSize(c, 12)).FontWeight(600).TextColor(flagColor(t, 1))
			}
		})
		if writable && frostui.IconButton(c, "document-edit", "Edit Event").Clicked() {
			m.editSelected()
		}
		if writable && !invited && frostui.IconButton(c, "user-trash", "Delete Event").Clicked() {
			m.deleteSelected()
		}
	})
	ui.Column(c).AlignItems(ui.Stretch).Children(func() {
		frostui.Body(c, k.dateText(e.AllDay, e.Start, e.StartDate, e.EndDate)).Selectable()
		if e.AllDay {
			frostui.Body(c, "All day")
		} else {
			frostui.Body(c, k.timeRange(e.Start, e.End)).Selectable()
			if e.TimeZone != "" && e.TimeZone != m.zone {
				if loc := loadZone(e.TimeZone); loc != nil {
					there := clock{loc: loc, h24: k.h24}
					ui.Text(c, e.TimeZone+": "+there.timeRange(e.Start, e.End)).FontSize(frostui.TextSize(c, 12)).TextColor(tertiary(t)).Selectable()
				}
			}
		}
	})
	ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Children(func() {
		if text := describeRecurrence(e.Recurrence); text != "" {
			labeled(c, "Repeats", 80, func() { frostui.Body(c, text).Wrap() })
		}
		if col := m.collection(e.CalendarID); col != nil {
			account := m.accountEmail(col.AccountID)
			labeled(c, "Calendar", 80, func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Tooltip(col.Name + " · " + account).Children(func() {
					uikit.Dot(c, 8, calColor(t, col.Color))
					nameAndAccount(c, t, col.Name, account)
				})
			})
		}
		if len(e.Alarms) > 0 {
			labeled(c, "Alerts", 80, func() {
				for i, a := range e.Alarms {
					frostui.Body(c.Key(i), alarmText(a))
				}
			})
		}
		if e.Answer != nil {
			labeled(c, "Your answer", 80, func() { frostui.Body(c, answerText(*e.Answer)) })
		}
	})
	if e.Organizer != nil {
		m.peopleBlock(c, "Organizer", []api.Attendee{*e.Organizer})
	}
	m.peopleBlock(c, "Invitees", e.Attendees)
	if e.Description != "" {
		frostui.Body(c, e.Description).Wrap().Selectable()
	}
}

func (m *Module) peopleBlock(c *ui.Context, title string, people []api.Attendee) {
	if len(people) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c.Key(title)).AlignItems(ui.Stretch).Role(ui.RoleList).Label(title).Children(func() {
		frostui.Label(c, title).Padding(0, 0, frostui.SpaceS, 0)
		for _, p := range people {
			ui.Row(c.Key(p.Email)).Height(36).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				avatar(c, 28, p.Name, p.Email, nil)
				ui.Row(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
					name := p.Name
					if name == "" {
						name = p.Email
					}
					if textButton(c, name, t.Accent, 13).Clicked() {
						m.showCard(p.Email, p.Name)
					}
					if p.IsUser {
						frostui.Body(c, " (you)")
					}
					if p.Role == api.AttendeeRoleOptional {
						frostui.Body(c, " (optional)").TextColor(tertiary(t))
					}
				})
				answerIcon(c, p.Answer)
			})
		}
	})
}

// answerIcon draws an invitee's answer; nothing for no answer.
func answerIcon(c *ui.Context, a api.PartStat) {
	t := c.Theme()
	switch a {
	case api.PartStatAccepted:
		frostui.Icon(c, "object-select").FontSize(frostui.TextSize(c, 14)).TextColor(flagColor(t, 4)).Label("Accepted")
	case api.PartStatDeclined:
		frostui.Icon(c, "window-close").FontSize(frostui.TextSize(c, 14)).TextColor(flagColor(t, 1)).Label("Declined")
	case api.PartStatTentative:
		frostui.Icon(c, "dialog-question").FontSize(frostui.TextSize(c, 14)).TextColor(flagColor(t, 2)).Label("Maybe")
	}
}

// showCard opens an address's contact card; without one to open, People
// on the address's person, or a message when they have none.
func (m *Module) showCard(email, name string) {
	if m.o.ContactCard != nil {
		m.o.ContactCard(email, name)
		return
	}
	c := m.client
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		card, err := c.People().Card(ctx, &api.PeopleCardParams{Email: email})
		m.o.Post(func() {
			if err != nil {
				slog.Warn("contact card", "err", err)
				return
			}
			if card.Person != nil {
				m.ShowPerson(card.Person.ID)
			} else if m.o.Compose != nil {
				m.o.Compose(email, card.Name)
			}
		})
	}()
}

// deleteSelected deletes the selection: through the scope sheet for an
// occurrence of a series, at once otherwise.
func (m *Module) deleteSelected() {
	e := m.shownEvent()
	if e == nil {
		return
	}
	if writable, invited := m.eventRights(e); !writable || invited {
		return
	}
	id, rid := e.ID, e.RecurrenceID
	send := func(scope *api.EventScope) {
		m.withNotify(m.o.Dialogs, func(ctx context.Context, c *api.Client, notify *bool) (string, error) {
			p := &api.CalendarDeleteParams{ID: id, Scope: scope, Notify: notify}
			if scope != nil {
				p.RecurrenceID = &rid
			}
			w, err := c.Calendar().Delete(ctx, p)
			if err != nil {
				return "", err
			}
			return w.RestoreID, nil
		}, func(o writeOutcome) {
			if o.err != nil {
				m.o.Toast("Could not delete the event", o.err)
				return
			}
			if !o.cancelled {
				m.rememberCalendar(o.restore)
				if sel := m.cal.selected; sel != nil && sel.eventID == id && sel.recurrenceID == rid {
					m.cal.selected = nil
				}
			}
		})
	}
	if e.Recurring && e.RecurrenceID != "" {
		m.cal.scope = &scopeSheet{delete: true, first: e.First != nil && *e.First, choose: func(s api.EventScope) { send(&s) }}
		return
	}
	send(nil)
}

// dropOccurrence writes a drop, after the scope sheet for a series.
func (m *Module) dropOccurrence(o api.Occurrence, fields func(zone string) api.EventFields) {
	if !canMove(&o, m.writableCalendars()) || m.client == nil {
		return
	}
	c, daemon := m.client, m.daemon
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		p := &api.CalendarEventParams{ID: o.EventID}
		if o.RecurrenceID != "" {
			p.RecurrenceID = &o.RecurrenceID
		}
		e, err := c.Calendar().Event(ctx, p)
		m.o.Post(func() {
			if m.daemon != daemon {
				return
			}
			if err != nil {
				m.o.Toast("Could not move the event", err)
				return
			}
			zone := e.TimeZone
			if zone == "" {
				zone = m.zone
			}
			f := fields(zone)
			if e.Recurring && e.RecurrenceID != "" {
				m.cal.scope = &scopeSheet{first: e.First != nil && *e.First, choose: func(s api.EventScope) { m.updateEvent(e, &s, f) }}
				return
			}
			m.updateEvent(e, nil, f)
		})
	}()
}

func (m *Module) updateEvent(e *api.CalendarEvent, scope *api.EventScope, f api.EventFields) {
	id, rid := e.ID, e.RecurrenceID
	m.withNotify(m.o.Dialogs, func(ctx context.Context, c *api.Client, notify *bool) (string, error) {
		p := &api.CalendarUpdateParams{ID: id, Fields: f, Scope: scope, Notify: notify}
		if scope != nil {
			p.RecurrenceID = &rid
		}
		w, err := c.Calendar().Update(ctx, p)
		if err != nil {
			return "", err
		}
		return w.RestoreID, nil
	}, func(o writeOutcome) {
		if o.err != nil {
			m.o.Toast("Could not move the event", o.err)
			return
		}
		if !o.cancelled {
			m.rememberCalendar(o.restore)
		}
	})
}
