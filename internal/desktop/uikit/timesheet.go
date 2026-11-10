package uikit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// TimeSheet asks for a future date and time. Keep one in the window's
// state, Show it, and call View every frame.
type TimeSheet struct {
	open   bool
	title  string
	at     time.Time
	choose func(time.Time)
}

// Show opens the sheet at the time given; choose gets the time on OK.
func (s *TimeSheet) Show(title string, at time.Time, choose func(time.Time)) {
	s.open, s.title, s.at, s.choose = true, title, at, choose
}

// View draws the sheet while it is open; now decides what is in the future.
func (s *TimeSheet) View(c *ui.Context, now time.Time) {
	if !s.open {
		return
	}
	ui.Modal(c, &s.open, func() {
		ui.Column(c).Width(360).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label(s.title).Children(func() {
			frostui.Heading(c, s.title)
			ui.Row(c).Gap(frostui.SpaceS).Children(func() {
				ui.DateInput(c, &s.at).Label("Date").Grow(1)
				ui.TimeInput(c, &s.at).Label("Time")
			})
			ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					s.open = false
				}
				if ui.PrimaryButton(c, "OK").Disabled(!s.at.After(now)).Clicked() {
					s.open = false
					s.choose(s.at)
				}
			})
		})
	})
}
