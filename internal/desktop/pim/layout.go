package pim

import (
	"cmp"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
)

// timedBlock is one day's part of a timed occurrence.
type timedBlock struct {
	occ             *api.Occurrence
	date            string
	startMin        int
	endMin          int
	column, columns int
	before, after   bool // continues from the day before, into the day after
}

// allDayBar spans the shown days first to last, inclusive, in a lane.
type allDayBar struct {
	occ         *api.Occurrence
	lane        int
	first, last int
}

func declined(o *api.Occurrence) bool {
	return o.Answer != nil && *o.Answer == api.PartStatDeclined
}

// overlaps reports whether a timed occurrence touches the instants
// [from, to); a zero-length one counts within them.
func overlaps(o *api.Occurrence, from, to time.Time) bool {
	if !o.Start.Before(to) {
		return false
	}
	if o.Start.Equal(o.End) {
		return !o.Start.Before(from)
	}
	return o.End.After(from)
}

func dayBlocks(occs []api.Occurrence, date string, loc *time.Location) []timedBlock {
	from, to := dayStart(date, loc), dayStart(addDays(date, 1), loc)
	var out []timedBlock
	for i := range occs {
		o := &occs[i]
		if o.AllDay || declined(o) || !overlaps(o, from, to) {
			continue
		}
		b := timedBlock{occ: o, date: date, endMin: 1440, columns: 1, before: o.Start.Before(from), after: o.End.After(to)}
		if !b.before {
			_, b.startMin = zoned(o.Start, loc)
		}
		if o.End.Before(to) {
			_, b.endMin = zoned(o.End, loc)
		}
		out = append(out, b)
	}
	slices.SortStableFunc(out, func(a, b timedBlock) int {
		return cmp.Or(cmp.Compare(a.startMin, b.startMin), cmp.Compare(b.endMin, a.endMin))
	})
	return out
}

// assignColumns gives each block a column within its cluster of
// overlapping blocks. A short block takes the room its 18-high minimum
// needs: 22.5 minutes.
func assignColumns(blocks []timedBlock) []timedBlock {
	const minRoom = 22.5
	var cluster []int
	var ends []float64
	latest := float64(-1 << 30)
	finish := func() {
		for _, i := range cluster {
			blocks[i].columns = len(ends)
		}
	}
	for i := range blocks {
		b := &blocks[i]
		start := float64(b.startMin)
		if start >= latest {
			finish()
			cluster, ends = cluster[:0], ends[:0]
		}
		at := slices.IndexFunc(ends, func(end float64) bool { return end <= start })
		if at < 0 {
			at = len(ends)
			ends = append(ends, 0)
		}
		b.column = at
		room := max(float64(b.endMin), start+minRoom)
		ends[at] = room
		latest = max(latest, room)
		cluster = append(cluster, i)
	}
	finish()
	slices.SortStableFunc(blocks, func(a, b timedBlock) int {
		return cmp.Or(cmp.Compare(a.startMin, b.startMin), cmp.Compare(a.column, b.column))
	})
	return blocks
}

// timedLayout splits timed occurrences by day and assigns their columns.
func timedLayout(occs []api.Occurrence, days []string, loc *time.Location) map[string][]timedBlock {
	out := make(map[string][]timedBlock, len(days))
	for _, date := range days {
		out[date] = assignColumns(dayBlocks(occs, date, loc))
	}
	return out
}

// allDayLanes gives each all-day bar the lowest free lane.
func allDayLanes(occs []api.Occurrence, days []string) []allDayBar {
	if len(days) == 0 {
		return nil
	}
	from, to := days[0], addDays(days[len(days)-1], 1)
	var bars []allDayBar
	for i := range occs {
		o := &occs[i]
		if !o.AllDay || declined(o) || o.StartDate >= to || o.EndDate <= from {
			continue
		}
		first := slices.IndexFunc(days, func(d string) bool { return d >= o.StartDate })
		last := len(days) - 1
		for last >= 0 && days[last] >= o.EndDate {
			last--
		}
		bars = append(bars, allDayBar{occ: o, first: first, last: last})
	}
	slices.SortStableFunc(bars, func(a, b allDayBar) int {
		return cmp.Or(cmp.Compare(a.first, b.first), cmp.Compare(b.last, a.last))
	})
	var ends []int
	for i := range bars {
		at := slices.IndexFunc(ends, func(end int) bool { return end < bars[i].first })
		if at < 0 {
			at = len(ends)
			ends = append(ends, 0)
		}
		bars[i].lane = at
		ends[at] = bars[i].last
	}
	return bars
}

// monthItems lists a day's occurrences: all-day ones first, then timed
// ones by start.
func monthItems(occs []api.Occurrence, date string, loc *time.Location) []*api.Occurrence {
	from, to := dayStart(date, loc), dayStart(addDays(date, 1), loc)
	var out []*api.Occurrence
	for i := range occs {
		o := &occs[i]
		if declined(o) {
			continue
		}
		if (o.AllDay && o.StartDate <= date && date < o.EndDate) || (!o.AllDay && overlaps(o, from, to)) {
			out = append(out, o)
		}
	}
	slices.SortStableFunc(out, func(a, b *api.Occurrence) int {
		if a.AllDay != b.AllDay {
			if a.AllDay {
				return -1
			}
			return 1
		}
		if a.AllDay {
			return cmp.Or(cmp.Compare(a.StartDate, b.StartDate), cmp.Compare(b.EndDate, a.EndDate))
		}
		return a.Start.Compare(b.Start)
	})
	return out
}

// busyDates is the days with occurrences that show.
func busyDates(occs []api.Occurrence, days []string, loc *time.Location) map[string]bool {
	out := make(map[string]bool, len(days))
	for _, date := range days {
		if len(monthItems(occs, date, loc)) > 0 {
			out[date] = true
		}
	}
	return out
}
