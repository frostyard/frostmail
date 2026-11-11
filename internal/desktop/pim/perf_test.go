package pim

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
)

// seedMonth stores 400 events over October 2026.
func (f *fixture) seedMonth(tb testing.TB) {
	ctx := tb.Context()
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		for i := range 400 {
			day := 1 + i%31
			hour := 7 + (i/31)%12
			start := time.Date(2026, 10, day, hour, 15*(i%4), 0, 0, time.UTC)
			end := start.Add(45 * time.Minute)
			raw := ics(fmt.Sprintf("UID:load-%d", i), "DTSTART:"+start.Format("20060102T150405Z"), "DTEND:"+end.Format("20060102T150405Z"), fmt.Sprintf("SUMMARY:Load %d", i))
			if _, err := pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: f.work, Href: fmt.Sprintf("/work/load-%d.ics", i), ETag: `"1"`, Kind: store.ObjectVEvent, Raw: raw},
				calendar.Options{Local: time.UTC}, from, to); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

// loadedWeek is a fixture whose week view shows a 400-event month.
func loadedWeek(t *testing.T) *fixture {
	f := newFixture(t, commands.Calendar)
	f.seedMonth(t)
	f.until("the load", func() bool { return f.tt.HasText("Load 0") || len(f.m.cal.occ.value) > 50 })
	f.m.cal.occ.dirty = true
	f.until("the whole week", func() bool { return !f.m.cal.occ.dirty && len(f.m.cal.occ.value) > 80 })
	return f
}

// frameLimit is plan 0015's gate for a frame of the week view (Phase 7).
const frameLimit = 16 * time.Millisecond

// The week view of a 400-event month (plan 0015, Phase 7: under 16 ms on
// the CI runner). The test fails when the median frame is over the limit;
// the race detector slows every frame, so it does not run under it.
func TestWeekViewOfA400EventMonthFrameTime(t *testing.T) {
	if raceDetector {
		t.Skip("the race detector's frames say nothing of the gate")
	}
	f := loadedWeek(t)
	shown := len(f.m.cal.occ.value)
	const frames = 200
	samples := make([]time.Duration, frames)
	before := f.builds
	for i := range frames {
		start := time.Now()
		f.tt.Frame()
		samples[i] = time.Since(start)
	}
	if built := f.builds - before; built < frames {
		t.Fatalf("%d frames built the view only %d times", frames, built)
	}
	slices.Sort(samples)
	var sum time.Duration
	for _, s := range samples {
		sum += s
	}
	t.Logf("week view, %d occurrences in the week, %d frames at 1200x780: min %v, median %v, mean %v, p95 %v, max %v", shown, frames, samples[0], samples[frames/2], sum/frames, samples[frames*95/100], samples[frames-1])
	if median := samples[frames/2]; median > frameLimit {
		t.Errorf("the median frame took %v, over the gate's %v", median, frameLimit)
	}
	// A frame that lays the week out again, as after calendar.changed.
	for i := range frames {
		f.m.cal.layoutKey = ""
		start := time.Now()
		f.tt.Frame()
		samples[i] = time.Since(start)
	}
	slices.Sort(samples)
	t.Logf("with the week laid out again each frame: median %v, p95 %v, max %v", samples[frames/2], samples[frames*95/100], samples[frames-1])
	// A frame after a resize, which lays out and shapes again.
	for i := range frames {
		start := time.Now()
		f.tt.SetSize(1200+i%2, 780)
		samples[i] = time.Since(start)
	}
	slices.Sort(samples)
	t.Logf("with the window resized each frame: median %v, p95 %v, max %v", samples[frames/2], samples[frames*95/100], samples[frames-1])
	// Painted to pixels on the CPU, as a snapshot is.
	for i := range 20 {
		start := time.Now()
		f.tt.Frame()
		_ = f.tt.Image()
		samples[i] = time.Since(start)
	}
	slices.Sort(samples[:20])
	t.Logf("with the frame's image read back: median %v, max %v", samples[10], samples[19])
}
