package pim

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// The time grid's sizes (docs/specs/pim-ui.md, Day and week views).
const (
	gutterWidth  = 56
	gridHeight   = 24 * hourHeight
	laneHeight   = 22
	laneGap      = 2
	minBlock     = 18
	blockGap     = 4
	monthLineH   = 18
	monthCellPad = 32
	barInset     = 2
)

// gridPress is a press in the time grid, while it lasts.
type gridPress struct {
	kind     string // "draw", "move" or "resize"
	owner    string // the element it began on
	occ      api.Occurrence
	day      int
	date     string
	y        float32 // where a draw began, in the column
	dx, dy   float32
	width    float32
	dragging bool
	canceled bool
	preview  *span
	seen     bool
}

// barGrab is a press on an all-day bar, while it lasts: the day it began
// on, which a drop measures from.
type barGrab struct {
	key  occKey
	date string
	seen bool
}

// dayDrop is the value a month or all-day drag carries.
type dayDrop struct {
	occ  api.Occurrence
	date string
}

func (m *Module) calendarView(c *ui.Context) {
	cal := &m.cal
	t := c.Theme()
	view := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Focusable().TabStop(false).FocusRing(false).Background(t.Background)
	if cal.focusAsk == paneCalView {
		cal.focusAsk = ""
		view.Focus()
	}
	if view.FocusWithin() {
		cal.focus = paneCalView
	}
	view.Children(func() {
		days := viewDays(cal.view, cal.date, m.clock.weekStart)
		if cal.view == viewMonth {
			cal.timeGridOn = false
			m.monthGrid(c, days)
		} else {
			if !cal.timeGridOn {
				cal.timeGridOn = true
				cal.scroll.Y = 7*hourHeight - 12
			}
			m.timeGrid(c, days)
		}
		drawScope(c, &cal.scope)
	})
}

// focusView keeps the keyboard in the view after a press inside it.
func (m *Module) focusView() { m.cal.focusAsk = paneCalView }

// eventLook is an occurrence's fill, text color and whether its border is
// dashed (docs/specs/pim-ui.md, Calendar module: What shows).
func eventLook(t *ui.Theme, color ui.Color, o *api.Occurrence, selected bool) (fill, text ui.Color, dashed bool) {
	switch {
	case selected:
		return color, t.AccentText, false
	case o.Answer != nil && (*o.Answer == api.PartStatNeedsaction || *o.Answer == api.PartStatTentative):
		return t.Background, t.Text, true
	}
	return t.Background.Mix(color, 0.18), t.Text, false
}

func (m *Module) isSelected(o *api.Occurrence) bool {
	return m.cal.selected != nil && *m.cal.selected == keyOf(o)
}

func title(o *api.Occurrence) string {
	if o.Summary != "" {
		return o.Summary
	}
	return "No Title"
}

func (m *Module) timeGrid(c *ui.Context, days []string) {
	cal := &m.cal
	if cal.press != nil {
		cal.press.seen = false
	}
	if cal.grab != nil {
		cal.grab.seen = false
	}
	m.dayHeaders(c, days)
	m.allDayStrip(c, days)
	ui.Scroll(c).Grow(1).Fill().TrackScroll(&cal.scroll).Children(func() {
		ui.Row(c).Height(gridHeight).AlignItems(ui.Stretch).Children(func() {
			m.gutter(c, days)
			for i, date := range days {
				m.dayColumn(c.Key(date), i, date, days)
			}
		})
	})
	if cal.press != nil && !cal.press.seen {
		cal.press = nil
	}
	if cal.grab != nil && !cal.grab.seen {
		cal.grab = nil
	}
}

func (m *Module) dayHeaders(c *ui.Context, days []string) {
	t := c.Theme()
	today := m.today()
	ui.Row(c).Height(48).Shrink(0).AlignItems(ui.Stretch).Children(func() {
		ui.Box(c).Width(gutterWidth).Shrink(0)
		for _, date := range days {
			b := ui.ButtonBase(c.Key(date)).Column().Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(longDate(date))
			if date == today {
				b.Current(ui.CurrentDate)
			}
			if b.Clicked() {
				m.showDay(date)
			}
			b.Children(func() {
				ui.Text(c, dateOf(date).Format("Mon")).FontSize(11).TextColor(t.TextMuted).SingleLine()
				n := ui.Box(c).Size(24, 24).Radius(12).Center()
				color := t.Text
				if date == today {
					n.Background(t.Accent)
					color = t.AccentText
				}
				n.Children(func() { ui.Text(c, strconv.Itoa(dateOf(date).Day())).FontSize(15).TextColor(color).SingleLine() })
			})
		}
	})
	frostui.RowDivider(c)
}

func (m *Module) allDayStrip(c *ui.Context, days []string) {
	t := c.Theme()
	cal := &m.cal
	bars := cal.bars
	lanes := 0
	for i := range bars {
		lanes = max(lanes, bars[i].lane+1)
	}
	collapse := len(days) == 7 && lanes > 3
	shown := lanes
	if collapse {
		shown = 3
	}
	height := float32(8)
	if lanes > 0 {
		height = float32(shown)*(laneHeight+laneGap) + 6
	}
	ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Stretch).Children(func() {
		ui.Box(c).Width(gutterWidth).Shrink(0).Padding(4, 8, 0, 0).Children(func() {
			if lanes > 0 {
				ui.Text(c, "all-day").FontSize(11).TextColor(tertiary(t)).TextAlign(ui.End).SingleLine()
			}
		})
		strip := ui.Box(c).Grow(1).Basis(0).MinWidth(0)
		width := strip.LayoutBounds().W
		if width <= 0 {
			c.AnimationFrame()
		}
		cw := width / float32(len(days))
		strip.Children(func() {
			for i, date := range days {
				m.allDayCell(c.Key(date), i, date, cw, height)
			}
			for i := range bars {
				if !collapse || bars[i].lane < 2 {
					m.allDayBar(c.Key(keyOf(bars[i].occ)), &bars[i], days, cw)
				}
			}
			if collapse {
				m.allDayMore(c, bars, days, cw)
			}
		})
	})
	frostui.RowDivider(c)
}

// allDayCell is a day's empty space in the strip: double click creates,
// and a dragged bar drops there.
func (m *Module) allDayCell(c *ui.Context, day int, date string, cw, height float32) {
	t := c.Theme()
	cell := ui.Box(c).Absolute().Top(0).Left(float32(day) * cw).Width(cw).Height(height).Focusable().TabStop(false).FocusRing(false).Label("All-day " + longDate(date))
	if _, over := ui.DragOver[dayDrop](cell); over {
		cell.Border(2, t.Accent)
	}
	if v, ok := ui.Drop[dayDrop](cell); ok {
		m.dropDays(v, date)
	}
	if cell.DoubleClicked() {
		m.newAllDay(date)
	}
}

func (m *Module) dropDays(v dayDrop, date string) {
	days := daysBetween(v.date, date)
	if days == 0 {
		return
	}
	o := v.occ
	loc := m.clock.loc
	m.dropOccurrence(o, func(zone string) api.EventFields { return shiftedDays(&o, days, loc, zone) })
}

func (m *Module) allDayBar(c *ui.Context, bar *allDayBar, days []string, cw float32) {
	t := c.Theme()
	o := bar.occ
	active := m.isSelected(o)
	color := m.colorOf(t, o.CalendarID)
	fill, text, dashed := eventLook(t, color, o, active)
	span := bar.last - bar.first + 1
	b := ui.ButtonBase(c).Row().Absolute().Top(4+float32(bar.lane)*(laneHeight+laneGap)).Left(float32(bar.first)*cw+barInset).
		Width(float32(span)*cw-2*barInset).Height(laneHeight).Radius(4).Padding(0, 4).AlignItems(ui.Center).Background(fill).KeepFocus().Clip().
		Cursor(ui.CursorPointer).Label(title(o) + ", all day").Checked(active)
	if dashed {
		b.Border(1, color).BorderStyle(ui.BorderDashed)
	}
	if o.Status == api.EventStatusCancelled {
		b.Opacity(0.6)
	}
	if canMove(o, m.cal.writable) {
		b.Drag(dayDrop{*o, m.grabbedDay(b, bar, days, cw)})
	}
	if b.Clicked() {
		m.selectOccurrence(o)
		m.focusView()
	}
	if b.DoubleClicked() {
		m.selectOccurrence(o)
		m.editSelected()
	}
	b.Children(func() {
		name := ui.Text(c, title(o)).FontSize(12).FontWeight(600).TextColor(text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if o.Status == api.EventStatusCancelled {
			name.Strikethrough()
		}
	})
}

// grabbedDay is the day a drag of a bar measures from: the one the press
// began on, read once, whichever of the bar's days the pointer crosses
// after.
func (m *Module) grabbedDay(b ui.Element, bar *allDayBar, days []string, cw float32) string {
	cal := &m.cal
	key := keyOf(bar.occ)
	dx, _, held := b.Dragged()
	if !held {
		return days[bar.first]
	}
	if g := cal.grab; g != nil && g.key == key {
		g.seen = true
		return g.date
	}
	x, _, _ := b.PointerPosition()
	cal.grab = &barGrab{key: key, date: barDay(bar, days, x-dx, cw), seen: true}
	return cal.grab.date
}

// barDay is the shown day under a point x pixels into a bar.
func barDay(bar *allDayBar, days []string, x, cw float32) string {
	if cw <= 0 {
		return days[bar.first]
	}
	return days[min(max(bar.first+int((x+barInset)/cw), 0), len(days)-1)]
}

func (m *Module) allDayMore(c *ui.Context, bars []allDayBar, days []string, cw float32) {
	t := c.Theme()
	for i, date := range days {
		n := 0
		for j := range bars {
			if bars[j].lane >= 2 && bars[j].first <= i && bars[j].last >= i {
				n++
			}
		}
		if n == 0 {
			continue
		}
		b := ui.ButtonBase(c.Key([2]any{"more", date})).Absolute().Top(4 + 2*(laneHeight+laneGap)).Left(float32(i)*cw + 2).Height(laneHeight).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label(fmt.Sprintf("Show %d more on %s", n, longDate(date)))
		if b.Clicked() {
			m.showDay(date)
		}
		b.Children(func() { ui.Text(c, fmt.Sprintf("%d more", n)).FontSize(11).TextColor(t.TextMuted).SingleLine() })
	}
}

func (m *Module) gutter(c *ui.Context, days []string) {
	t := c.Theme()
	k := m.clock
	today := m.today()
	now := m.now()
	shown := false
	for _, d := range days {
		shown = shown || d == today
	}
	ui.Box(c).Width(gutterWidth).Shrink(0).Children(func() {
		for hour := 1; hour < 24; hour++ {
			ui.Text(c.Key(hour), hourLabel(hour, k.h24)).FontSize(11).TextColor(tertiary(t)).Absolute().Top(float32(hour*hourHeight) - 7).Right(8).SingleLine()
		}
		if shown {
			_, minutes := zoned(now, k.loc)
			ui.Text(c.Key("now"), k.clockTime(now)).FontSize(11).FontWeight(600).TextColor(flagColor(t, 1)).Background(t.Background).Absolute().Top(float32(minutes)*pxPerMinute - 7).Right(8).SingleLine()
		}
	})
}

func hourLabel(hour int, h24 bool) string {
	if h24 {
		return fmt.Sprintf("%02d:00", hour)
	}
	switch {
	case hour == 12:
		return "12 PM"
	case hour > 12:
		return strconv.Itoa(hour-12) + " PM"
	}
	return strconv.Itoa(hour) + " AM"
}

func (m *Module) dayColumn(c *ui.Context, day int, date string, days []string) {
	t := c.Theme()
	cal := &m.cal
	col := ui.Column(c).Grow(1).Basis(0).MinWidth(0).Role(ui.RoleGroup).Label(longDate(date))
	width := col.LayoutBounds().W
	if width <= 0 {
		c.AnimationFrame()
	}
	sep := t.Border
	col.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 1, H: r.H}, sep, 0)
		for hour := range 24 {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + float32(hour*hourHeight), W: r.W, H: 1}, sep, 0)
		}
	})
	m.columnGesture(col, day, date, days, width)
	col.Children(func() {
		for i := range cal.blocks[date] {
			m.eventBlock(c.Key(keyOf(cal.blocks[date][i].occ)), &cal.blocks[date][i], day, width, days)
		}
		if p := cal.press; p != nil && p.preview != nil {
			if d, _ := zoned(p.preview.start, m.clock.loc); d == date {
				m.dragPreview(c, p, width)
			}
		}
		if date == m.today() {
			_, minutes := zoned(m.now(), m.clock.loc)
			now := flagColor(t, 1)
			top := float32(minutes) * pxPerMinute
			ui.Box(c.Key("now")).Absolute().Top(top).Left(0).Right(0).Height(2).Background(now).IgnoreInput(true).Label("Now")
			ui.Box(c.Key("now-dot")).Absolute().Top(top-3).Left(-4).Size(8, 8).Radius(4).Background(now).IgnoreInput(true)
		}
	})
}

// columnGesture follows a press on a column's empty time: a drag draws a
// new event, and a double click makes one at that half hour.
func (m *Module) columnGesture(col ui.Element, day int, date string, days []string, width float32) {
	cal := &m.cal
	owner := "column:" + date
	dx, dy, held := col.Dragged()
	if held && cal.press == nil && len(cal.writable) > 0 {
		_, y, _ := col.PointerPosition()
		cal.press = &gridPress{kind: "draw", owner: owner, day: day, date: date, y: y - dy, width: width}
		m.focusView()
	}
	p := cal.press
	if p != nil && p.owner == owner {
		p.seen = true
		switch {
		case held:
			p.dx, p.dy = p.dx+dx, p.dy+dy
			m.updatePreview(p)
		case col.PressCanceled():
			cal.press = nil
		default:
			cal.press = nil
			if !p.canceled && p.dragging && p.preview != nil {
				m.newTimed(*p.preview)
			}
		}
	}
	if col.DoubleClicked() {
		if _, y, ok := col.PointerPosition(); ok {
			start := int(y/pxPerMinute) / 30 * 30
			m.newTimed(span{instantAt(date, start, m.clock.loc), instantAt(date, start+60, m.clock.loc)})
		}
	}
}

// updatePreview follows the pointer: past the threshold a press is a drag,
// which shows its range in steps.
func (m *Module) updatePreview(p *gridPress) {
	if p.canceled {
		return
	}
	if !p.dragging && max(abs32(p.dx), abs32(p.dy)) < dragThresh {
		return
	}
	p.dragging = true
	loc := m.clock.loc
	var r span
	switch p.kind {
	case "draw":
		r = drawn(p.date, minuteAt(p.y), minuteAt(p.y+p.dy), loc)
	case "resize":
		r = resized(span{p.occ.Start, p.occ.End}, stepsOf(p.dy), loc)
	default:
		days := len(viewDays(m.cal.view, m.cal.date, m.clock.weekStart))
		d := max(-p.day, min(days-1-p.day, daysOf(p.dx, p.width)))
		r = moved(span{p.occ.Start, p.occ.End}, d, stepsOf(p.dy), loc)
	}
	p.preview = &r
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// eventBlock draws a timed occurrence's part of a day.
func (m *Module) eventBlock(c *ui.Context, block *timedBlock, day int, width float32, days []string) {
	t := c.Theme()
	cal := &m.cal
	o := block.occ
	active := m.isSelected(o)
	color := m.colorOf(t, o.CalendarID)
	fill, text, dashed := eventLook(t, color, o, active)
	height := max(float32(block.endMin-block.startMin)*pxPerMinute, minBlock)
	rng := m.clock.timeRange(o.Start, o.End)
	label := title(o) + ", " + rng
	if o.Location != "" {
		label += ", " + o.Location
	}
	columnW := width / float32(block.columns)
	b := ui.ButtonBase(c).Column().Absolute().Top(float32(block.startMin)*pxPerMinute).Left(float32(block.column)*columnW).
		Width(max(columnW-blockGap, 8)).Height(height).Radius(4).Padding(4, 4, 4, 7).Background(fill).AlignItems(ui.Stretch).KeepFocus().Clip().
		Label(label).Checked(active)
	if dashed {
		b.Border(1, color).BorderStyle(ui.BorderDashed)
	}
	if o.Status == api.EventStatusCancelled {
		b.Opacity(0.6)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 3, H: r.H}, color, 0) })
	ended := false
	movable := canMove(o, m.cal.writable)
	if movable {
		ended = m.blockGesture(b, block, day, width, height, days)
		if _, y, ok := b.PointerPosition(); ok && y >= height-resizeEdge {
			b.Cursor(ui.CursorResizeNS)
		} else {
			b.Cursor(ui.CursorPointer)
		}
	}
	if b.Clicked() && !ended && (cal.press == nil || !cal.press.dragging) {
		m.selectOccurrence(o)
		m.focusView()
	}
	if b.DoubleClicked() {
		m.selectOccurrence(o)
		m.editSelected()
	}
	b.Children(func() {
		name := ui.Text(c, title(o)).FontSize(12).FontWeight(600).TextColor(text).SingleLine()
		if o.Status == api.EventStatusCancelled {
			name.Strikethrough()
		}
		if height >= 36 {
			sub := t.TextMuted
			if active {
				sub = t.AccentText
			}
			ui.Text(c, rng).FontSize(11).TextColor(sub).SingleLine()
			if o.Location != "" {
				ui.Text(c, o.Location).FontSize(11).TextColor(sub).SingleLine()
			}
		}
	})
}

// blockGesture follows a press on a block; it reports a drag that ended
// and wrote, which is no click.
func (m *Module) blockGesture(b ui.Element, block *timedBlock, day int, width, height float32, days []string) (ended bool) {
	cal := &m.cal
	o := block.occ
	owner := fmt.Sprint("block:", o.EventID, ":", o.RecurrenceID, ":", block.date)
	dx, dy, held := b.Dragged()
	if held && cal.press == nil {
		kind := "move"
		if _, y, ok := b.PointerPosition(); ok && y-dy >= height-resizeEdge {
			kind = "resize"
		}
		cal.press = &gridPress{kind: kind, owner: owner, occ: *o, day: day, date: block.date, width: width}
	}
	p := cal.press
	if p == nil || p.owner != owner {
		return false
	}
	p.seen = true
	switch {
	case held:
		p.dx, p.dy = p.dx+dx, p.dy+dy
		m.updatePreview(p)
	case b.PressCanceled():
		cal.press = nil
	default:
		cal.press = nil
		if p.canceled {
			return true
		}
		if !p.dragging || p.preview == nil || (p.preview.start.Equal(p.occ.Start) && p.preview.end.Equal(p.occ.End)) {
			return false
		}
		r := *p.preview
		occ := p.occ
		m.dropOccurrence(occ, func(zone string) api.EventFields { return dropFields(r, zone) })
		return true
	}
	return false
}

func (m *Module) dragPreview(c *ui.Context, p *gridPress, width float32) {
	t := c.Theme()
	loc := m.clock.loc
	_, start := zoned(p.preview.start, loc)
	minutes := float32(p.preview.end.Sub(p.preview.start).Minutes())
	fill, text := t.Accent, t.AccentText
	name := "New Event"
	if p.kind != "draw" {
		color := m.colorOf(t, p.occ.CalendarID)
		fill, text, _ = eventLook(t, color, &p.occ, false)
		name = title(&p.occ)
	}
	ui.Column(c.Key("preview")).Absolute().Top(float32(start) * pxPerMinute).Left(0).Width(max(width-blockGap, 8)).Height(max(minutes*pxPerMinute, minBlock)).
		Radius(4).Padding(4).Background(fill).Opacity(0.6).IgnoreInput(true).AlignItems(ui.Stretch).Clip().Children(func() {
		ui.Text(c, name).FontSize(12).FontWeight(600).TextColor(text).SingleLine()
		ui.Text(c, m.clock.timeRange(p.preview.start, p.preview.end)).FontSize(11).TextColor(text).SingleLine()
	})
}

func (m *Module) monthGrid(c *ui.Context, days []string) {
	t := c.Theme()
	cal := &m.cal
	lines := 4
	if cal.cellHeight > 0 {
		lines = max(2, int((cal.cellHeight-monthCellPad)/monthLineH))
	}
	ui.Row(c).Height(24).Shrink(0).AlignItems(ui.Center).Children(func() {
		for _, d := range days[:7] {
			ui.Text(c.Key(d), dateOf(d).Format("Mon")).FontSize(11).TextColor(t.TextMuted).Grow(1).Basis(0).TextAlign(ui.Center).SingleLine()
		}
	})
	ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Role(ui.RoleGrid).Label(viewTitle(viewMonth, cal.date, m.clock.weekStart)).Children(func() {
		for week := range 6 {
			row := ui.Row(c.Key(week)).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch)
			if h := row.LayoutBounds().H; h != cal.cellHeight {
				cal.cellHeight = h
				c.AnimationFrame()
			}
			row.Children(func() {
				for _, date := range days[week*7 : week*7+7] {
					m.monthCell(c.Key(date), date, lines)
				}
			})
		}
	})
}

func (m *Module) monthCell(c *ui.Context, date string, lines int) {
	t := c.Theme()
	cal := &m.cal
	today := m.today()
	items := cal.cells[date]
	visible := lines
	if len(items) > lines {
		visible = lines - 1
	}
	if visible > len(items) {
		visible = len(items)
	}
	more := len(items) - visible
	cell := ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(4).Gap(0).AlignItems(ui.Stretch).Clip().Focusable().TabStop(false).FocusRing(false).Role(ui.RoleCell).Label(longDate(date)).AccessSelected(date == cal.date)
	if date == today {
		cell.Current(ui.CurrentDate)
	}
	if date == cal.date {
		cell.Background(t.SurfaceHover)
	}
	width := cell.LayoutBounds().W
	sep := t.Border
	cell.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, sep, 0)
		p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, sep, 0)
	})
	if _, over := ui.DragOver[dayDrop](cell); over {
		cell.Border(2, t.Accent)
	}
	if v, ok := ui.Drop[dayDrop](cell); ok {
		m.dropDays(v, date)
	}
	if cell.Clicked() {
		cal.date = date
		m.focusView()
	}
	if cell.DoubleClicked() {
		m.newAllDay(date)
	}
	cell.Children(func() {
		ui.Row(c).Justify(ui.End).Margin(0, 0, 4, 0).Children(func() {
			label := strconv.Itoa(dateOf(date).Day())
			if date[8:] == "01" {
				label = dateOf(date).Format("Jan 2")
			}
			n := ui.Box(c).Height(20).MinWidth(20).Radius(10).Center().Padding(0, 4)
			color := t.Text
			switch {
			case date == today:
				n.Background(t.Accent)
				color = t.AccentText
			case date[:7] != cal.date[:7]:
				color = tertiary(t)
			}
			n.Children(func() { ui.Text(c, label).FontSize(12).TextColor(color).SingleLine() })
		})
		for _, o := range items[:visible] {
			m.monthItem(c.Key(keyOf(o)), o, date, width)
		}
		if more > 0 {
			b := ui.ButtonBase(c.Key("more")).Height(monthLineH).Cursor(ui.CursorPointer).Label(fmt.Sprintf("Show %d more on %s", more, longDate(date)))
			if b.Clicked() {
				m.showDay(date)
			}
			b.Children(func() { ui.Text(c, fmt.Sprintf("%d more", more)).FontSize(11).TextColor(t.TextMuted).SingleLine() })
		}
	})
}

func (m *Module) monthItem(c *ui.Context, o *api.Occurrence, date string, cellWidth float32) {
	t := c.Theme()
	active := m.isSelected(o)
	color := m.colorOf(t, o.CalendarID)
	start := ""
	if !o.AllDay {
		start = m.clock.clockTime(o.Start)
	}
	name := "all day"
	if start != "" {
		name = start
	}
	b := ui.ButtonBase(c).Row().Height(monthLineH).Gap(4).Padding(0, 4).Radius(4).AlignItems(ui.Center).KeepFocus().Cursor(ui.CursorPointer).
		Label(title(o) + ", " + name).Checked(active)
	fill, text, dashed := eventLook(t, color, o, active)
	if o.AllDay || active {
		b.Background(fill)
	}
	if dashed {
		b.Border(1, color).BorderStyle(ui.BorderDashed)
	}
	if o.Status == api.EventStatusCancelled {
		b.Opacity(0.6)
	}
	if canMove(o, m.cal.writable) {
		b.Drag(dayDrop{*o, date})
	}
	if b.Clicked() {
		m.selectOccurrence(o)
		m.focusView()
	}
	if b.DoubleClicked() {
		m.selectOccurrence(o)
		m.editSelected()
	}
	b.Children(func() {
		if !o.AllDay {
			ui.Box(c).Size(6, 6).Radius(3).Background(color).Shrink(0)
		}
		label := ui.Text(c, title(o)).FontSize(11).TextColor(text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if o.AllDay {
			label.FontWeight(600)
		}
		if o.Status == api.EventStatusCancelled {
			label.Strikethrough()
		}
		if !o.AllDay && cellWidth >= 144 {
			ui.Text(c, start).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
	})
}
