package pim

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// searchRow is a row of the results: a heading, or an occurrence.
type searchRow struct {
	heading string
	occ     *api.Occurrence
}

// calendarSearch is the Calendar module's search: the field's text, the
// query it became, and maild's answer as rows.
type calendarSearch struct {
	draft, query string
	token        int
	open         bool // the field shows in the bar
	focusAsk     bool
	focused      bool // the field held the keyboard last frame
	results      fetch[[]api.Occurrence]
	rowsKey      string
	rows         []searchRow
	row          int
	listUI       ui.ListState
}

// searching says whether the results show in place of the view.
func (m *Module) searching() bool { return m.cal.search.query != "" }

// openSearch shows the field in the bar and gives it the keyboard.
func (m *Module) openSearch() {
	m.cal.search.open, m.cal.search.focusAsk = true, true
}

// setSearch makes the field's text the query.
func (m *Module) setSearch() {
	s := &m.cal.search
	s.token++
	s.query = strings.TrimSpace(s.draft)
}

// endSearch empties and closes the field and shows the view again, on
// the date of the selection if the results gave one.
func (m *Module) endSearch() {
	s := &m.cal.search
	s.token++
	s.draft, s.query = "", ""
	s.open, s.focused = false, false
}

// syncSearch asks for the query's occurrences and lays them out as rows:
// the ones not yet ended in start order, then the earlier ones, the
// latest first. maild may be on another machine, so a zone with no name
// to send is never left to maild's (loadRange): the call asks in UTC and
// puts all-day occurrences at this zone's midnights here.
func (m *Module) syncSearch() {
	s := &m.cal.search
	if s.query == "" {
		return
	}
	query, zone, loc := s.query, m.zone, m.clock.loc
	want(m, &s.results, "search|"+zone+"|"+query, func(ctx context.Context, c *api.Client) ([]api.Occurrence, error) {
		p := &api.CalendarSearchParams{Query: query, TimeZone: &zone}
		if zone != localZone {
			return c.Calendar().Search(ctx, p)
		}
		utc := "UTC"
		p.TimeZone = &utc
		occs, err := c.Calendar().Search(ctx, p)
		if err != nil {
			return nil, err
		}
		for i := range occs {
			if o := &occs[i]; o.AllDay {
				o.Start, o.End = dayStart(o.StartDate, loc), dayStart(o.EndDate, loc)
			}
		}
		slices.SortStableFunc(occs, func(a, b api.Occurrence) int { return a.Start.Compare(b.Start) })
		return occs, nil
	})
	now := m.now().Truncate(time.Minute)
	if key := fmt.Sprint(s.results.version, "|", now.Unix()); key != s.rowsKey {
		s.rowsKey = key
		s.rows = searchRows(s.results.value, now)
	}
	s.row = -1
	if sel := m.cal.selected; sel != nil {
		s.row = slices.IndexFunc(s.rows, func(r searchRow) bool { return r.occ != nil && keyOf(r.occ) == *sel })
	}
}

// searchRows splits occurrences in start order at now.
func searchRows(occs []api.Occurrence, now time.Time) []searchRow {
	first := slices.IndexFunc(occs, func(o api.Occurrence) bool { return o.End.After(now) })
	if first < 0 {
		first = len(occs)
	}
	var rows []searchRow
	if first < len(occs) {
		rows = append(rows, searchRow{heading: "Upcoming"})
		for i := first; i < len(occs); i++ {
			rows = append(rows, searchRow{occ: &occs[i]})
		}
	}
	if first > 0 {
		rows = append(rows, searchRow{heading: "Earlier"})
		for i := first - 1; i >= 0; i-- {
			rows = append(rows, searchRow{occ: &occs[i]})
		}
	}
	return rows
}

// searchWhen writes a result's day, time, place and state.
func (k clock) searchWhen(o *api.Occurrence) string {
	var parts []string
	if o.AllDay {
		parts = append(parts, dateOf(o.StartDate).Format("Mon, Jan 2, 2006"), "All day")
	} else {
		parts = append(parts, o.Start.In(k.loc).Format("Mon, Jan 2, 2006"), k.timeRange(o.Start, o.End))
	}
	if o.Location != "" {
		parts = append(parts, o.Location)
	}
	if o.Status == api.EventStatusCancelled {
		parts = append(parts, "Cancelled")
	}
	return strings.Join(parts, " · ")
}

// searchSubtitle counts the results under the bar's title.
func (m *Module) searchSubtitle() string {
	s := &m.cal.search
	n := len(s.results.value)
	switch {
	case !s.results.loaded || s.results.pending:
		return "Searching…"
	case s.results.err != nil:
		return "The search failed"
	case n == 1:
		return "1 result"
	}
	return fmt.Sprintf("%d results", n)
}

// searchField ends the bar over the view while a search is open. The
// event pane's bar has no room for it beside the command field and the
// window's controls. Left empty, it closes.
func (m *Module) searchField(c *ui.Context) {
	s := &m.cal.search
	f := ui.SearchField(c, &s.draft).Width(220).Shrink(1).MinWidth(100).Label("Search Events").Placeholder("Search Events")
	if s.focusAsk {
		s.focusAsk = false
		f.Focus()
	} else if focused := f.Focused(); focused || !s.focused || s.draft != "" || s.query != "" {
		s.focused = focused
	} else {
		m.endSearch()
	}
	if f.Changed() {
		s.token++
		token := s.token
		time.AfterFunc(searchPause, func() {
			m.o.Post(func() {
				if token == s.token {
					m.setSearch()
				}
			})
		})
	}
	if f.Submitted() {
		m.setSearch()
	}
}

// searchResults draws the results in place of the view.
func (m *Module) searchResults(c *ui.Context) {
	cal, s := &m.cal, &m.cal.search
	t := c.Theme()
	if s.results.loaded && !s.results.pending && len(s.rows) == 0 {
		text := "No Results"
		if s.results.err != nil {
			text = "The search failed"
		}
		centered(c, text, 15)
		return
	}
	rows := s.rows
	s.listUI.Selected = &s.row
	s.listUI.Key = func(i int) any {
		if i >= len(rows) {
			return i
		}
		if o := rows[i].occ; o != nil {
			return keyOf(o)
		}
		return "heading:" + rows[i].heading
	}
	s.listUI.Header = func(i int) bool { return i < len(rows) && rows[i].occ == nil }
	s.listUI.Label = func(i int) string {
		if i < len(rows) && rows[i].occ != nil {
			return title(rows[i].occ) + ", " + m.clock.searchWhen(rows[i].occ)
		}
		return ""
	}
	list := uikit.SoftList(c, &s.listUI, len(rows), func(i int) { m.searchRow(c, rows[i], t) }).Label("Search results").Background(t.Background)
	if cal.focusAsk == paneCalView {
		cal.focusAsk = ""
		list.Focus()
	}
	if list.FocusWithin() {
		cal.focus = paneCalView
	}
	if list.Changed() && s.row >= 0 && s.row < len(rows) && rows[s.row].occ != nil {
		m.selectOccurrence(rows[s.row].occ)
	}
	if list.Submitted() {
		m.editSelected()
	}
}

func (m *Module) searchRow(c *ui.Context, row searchRow, t *ui.Theme) {
	if row.occ == nil {
		ui.Row(c).Height(24).Padding(0, frostui.SpaceM).AlignItems(ui.Center).Background(frostui.Chrome(t)).Children(func() {
			frostui.Label(c, row.heading)
		})
		return
	}
	o := row.occ
	sep := t.Border
	ui.Row(c).Height(44).Padding(0, frostui.SpaceM).Gap(10).AlignItems(ui.Center).Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, sep, 0)
	}).Children(func() {
		uikit.Dot(c, 8, m.colorOf(t, o.CalendarID))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			frostui.Strong(c, title(o)).SingleLine()
			frostui.Muted(c, m.clock.searchWhen(o)).SingleLine()
		})
	})
}
