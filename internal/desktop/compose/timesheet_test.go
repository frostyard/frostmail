package compose_test

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/internal/desktop/compose"
)

type timeSheetFixture struct {
	sheet  compose.TimeSheet
	now    time.Time
	chosen []time.Time
	tt     *ui.Tester
}

func newTimeSheet() *timeSheetFixture {
	f := &timeSheetFixture{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	f.tt = ui.NewTester(func(c *ui.Context) { f.sheet.View(c, f.now) }, 720, 560)
	return f
}

func (f *timeSheetFixture) show(at time.Time) {
	f.sheet.Show("Send Later", at, func(t time.Time) { f.chosen = append(f.chosen, t) })
	f.tt.Frame()
}

func (f *timeSheetFixture) ok(t *testing.T) ui.AccessInfo {
	t.Helper()
	info, ok := f.tt.AccessByRole(ui.RoleButton, "OK")
	if !ok {
		t.Fatalf("no OK in %q", f.tt.Texts())
	}
	return info
}

func TestTimeSheetRefusesAPastTime(t *testing.T) {
	f := newTimeSheet()
	f.show(f.now.Add(-time.Hour))
	ok := f.ok(t)
	if !ok.Disabled {
		t.Fatal("OK is enabled for a past time")
	}
	f.tt.ClickAt(ok.Bounds.X+ok.Bounds.W/2, ok.Bounds.Y+ok.Bounds.H/2)
	f.tt.Frame()
	if len(f.chosen) != 0 || !f.tt.HasText("Send Later") {
		t.Errorf("chosen %v, window shows %q", f.chosen, f.tt.Texts())
	}
	f.now = f.now.Add(-2 * time.Hour) // the same time is now ahead
	f.tt.Frame()
	if f.ok(t).Disabled {
		t.Error("OK is off for a future time")
	}
}

func TestTimeSheetOKChooses(t *testing.T) {
	f := newTimeSheet()
	at := f.now.Add(time.Hour)
	f.show(at)
	if err := f.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if len(f.chosen) != 1 || !f.chosen[0].Equal(at) {
		t.Errorf("chosen %v, want %v", f.chosen, at)
	}
	if f.tt.HasText("Send Later") {
		t.Errorf("the sheet stayed: %q", f.tt.Texts())
	}
}

func TestTimeSheetCancelDoesNotChoose(t *testing.T) {
	f := newTimeSheet()
	f.show(f.now.Add(time.Hour))
	if err := f.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if len(f.chosen) != 0 {
		t.Errorf("Cancel chose %v", f.chosen)
	}
	if f.tt.HasText("Send Later") {
		t.Errorf("the sheet stayed: %q", f.tt.Texts())
	}
}
