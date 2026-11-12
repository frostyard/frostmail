package settings

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// The move note's words (docs/specs/ui.md, First run). The steps are the
// user's to run: the app never reads, copies or stops the host's maild.
const (
	moveFound   = "This computer also has mail from a Frostmail installed outside Flatpak. Nothing moves by itself. To move it here, run these commands in a terminal:"
	moveSecrets = "Then open Frostmail again. The folder ~/frostmail-export holds your passwords in plain text: delete it once your accounts sync."

	// moveNoteFile, beside app.json, says this computer dismissed the
	// note.
	moveNoteFile = "move-note-dismissed"
)

func moveSteps() []string {
	return []string{
		"systemctl --user disable --now maild.service",
		"mailctl export ~/frostmail-export",
		"flatpak run --command=mailctl org.frostyard.Frostmail import ~/frostmail-export",
	}
}

// firstRun is the welcome's own state.
type firstRun struct {
	dismissed bool
}

func moveNoteDismissed(configFile string) bool {
	return configFile != "" && fileExists(filepath.Join(filepath.Dir(configFile), moveNoteFile))
}

func (w *Window) dismissMoveNote() {
	w.firstRun.dismissed = true
	path := filepath.Join(filepath.Dir(w.o.ConfigFile), moveNoteFile)
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err == nil {
		err = os.WriteFile(path, nil, 0o600)
	}
	if err != nil {
		w.toast("Could not remember that the note was dismissed", err)
	}
}

// NeedsFirstRun reports whether maild lists no account: the main window
// then shows FirstRun in place of its panes.
func (w *Window) NeedsFirstRun() bool {
	return w.cn != nil && w.data.loaded && len(w.data.accounts) == 0
}

func (w *Window) openSettings(pane string) {
	if w.o.OpenSettings != nil {
		w.o.OpenSettings(pane)
		return
	}
	w.Show(pane)
}

// FirstRun is the main window's welcome. The first time a connection
// finds no account it opens the settings window on Accounts itself;
// removing the last account later shows the welcome without opening it.
func (w *Window) FirstRun(c *ui.Context) {
	if w.autoOpen {
		w.autoOpen = false
		w.o.Post(func() { w.openSettings(PaneAccounts) })
	}
	here := w.onThisComputer()
	text := "Add a mail account to start."
	if here {
		text = "Add a mail account to start. Frostmail keeps its mail on this computer."
	}
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Background(c.Theme().Background).Padding(frostui.SpaceXL).Role(ui.RoleRegion).Label("Welcome").Children(func() {
		ui.Column(c).Width(420).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
			ui.Text(c, "Welcome to Frostmail").FontSize(frostui.TextSize(c, 17)).Bold().Role(ui.RoleHeading).Level(1)
			frostui.Muted(c, text).Wrap()
			ui.Row(c).Gap(frostui.SpaceS).Padding(frostui.SpaceM, 0, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "Add Account…").Clicked() {
					w.openSettings(PaneAccounts)
				}
				if here && ui.Button(c, "Connect to lodge…").Clicked() {
					w.openSettings(PaneServer)
				}
			})
			if here && w.facts.Flatpak && w.facts.HostMail && !w.firstRun.dismissed {
				w.moveNote(c)
			}
		})
	})
}

// moveNote says how to move a host install's mail into the Flatpak. It
// changes nothing: the move is the three commands, run by the user.
func (w *Window) moveNote(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Margin(frostui.SpaceXL, 0, 0, 0).Padding(frostui.SpaceM).Gap(frostui.SpaceS).Radius(frostui.RadiusChip).Background(frostui.ToneFill(t, frostui.Neutral)).
		AlignItems(ui.Stretch).Role(ui.RoleGroup).Label("Mail from another install").Children(func() {
		ui.Text(c, moveFound).FontSize(frostui.TextSize(c, 12)).Wrap()
		for i, step := range moveSteps() {
			ui.Row(c.Key(i)).Gap(frostui.SpaceXS).Children(func() {
				ui.Text(c, string(rune('1'+i))+".").FontSize(frostui.TextSize(c, 12))
				frostui.Mono(c, step).FontSize(frostui.TextSize(c, 12)).Wrap().Selectable().Grow(1).Shrink(1).MinWidth(0)
			})
		}
		ui.Text(c, moveSecrets).FontSize(frostui.TextSize(c, 12)).Wrap()
		ui.Row(c).Children(func() {
			if ui.Button(c, "Dismiss").Clicked() {
				w.dismissMoveNote()
			}
		})
	})
}
