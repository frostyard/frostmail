package main

import (
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The first run (docs/specs/ui.md, First run; ADR-0063). A store with no
// account shows a welcome in place of the modules, with the ways to get
// mail: an account, or, on this computer's maild, Lodge. The first time a
// connection finds no account the client opens the account's dialog
// itself. In the release Flatpak, where a host install has mail, the
// welcome also says how to move it.

const (
	// moveNoteKey is the device's choice to hide the move note.
	moveNoteKey       = "move-note"
	moveNoteDismissed = "dismissed"

	moveFound   = "This computer also has mail from a Frostmail installed outside Flatpak. Nothing moves by itself. To move it here, run these commands in a terminal:"
	moveSecrets = "Then open Frostmail again. The folder ~/frostmail-export holds your passwords in plain text: delete it once your accounts sync."
)

// moveSteps are the commands that move a host install's mail into the
// Flatpak, as the MyGo client's note gives them.
var moveSteps = [...]string{
	"systemctl --user disable --now maild.service",
	"mailctl export ~/frostmail-export",
	"flatpak run --command=mailctl org.frostyard.Frostmail import ~/frostmail-export",
}

// needsFirstRun reports whether maild answered and lists no account: the
// window then shows the welcome in place of its modules.
func (w *workspace) needsFirstRun() bool {
	w.state.Get()
	st := w.store.State()
	return w.online.Get() && st.Loaded && len(st.Accounts) == 0
}

// onThisComputer reports whether the client is a desktop's on this
// computer's maild, and not Lodge's.
func (w *workspace) onThisComputer() bool {
	return desktopServer && !w.session.Target().Remote()
}

// watchFirstRun opens the account's dialog the first time a connection
// finds no account: once for a connection, so again after a reconnection
// that still finds none, and never when the connection started with an
// account. Removing the last account later shows the welcome alone.
func (w *workspace) watchFirstRun(cx *core.Scope) {
	cx.Effect(func() {
		w.state.Get()
		if !w.online.Get() || !w.firstLook {
			return
		}
		st := w.store.State()
		if !st.Loaded {
			return
		}
		w.firstLook = false
		if len(st.Accounts) == 0 {
			cx.Post(w.startAccount)
		}
	})
}

// welcome is the first run's pane.
func (w *workspace) welcome(cx *core.Scope) core.Node {
	dismissed := core.NewSignal(cx, func() bool { v, _ := cx.Storage().GetItem(moveNoteKey); return v == moveNoteDismissed }())
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		here := w.onThisComputer()
		text := "Add a mail account to start."
		if here {
			text = "Add a mail account to start. Frostmail keeps its mail on this computer."
		}
		actions := []core.Node{ui.Button{Text: "Add Account…", OnPress: w.startAccount}}
		if here {
			actions = append(actions, ui.Button{Text: "Connect to lodge…", Variant: ui.ButtonSecondary, OnPress: func() {
				w.settingsStart = "server"
				w.navigate("module:Settings")
			}})
		}
		children := []core.Node{
			ui.Icon(lucide.Mail, 32, colors.Faint),
			core.Text{Content: "Welcome to Frostmail", Role: core.RoleHeader, Style: ui.TypeHeading},
			core.Text{Content: text, Style: ui.TypeBody.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
			core.View{Style: core.Style{Margin: core.Edges{Top: workspaceui.SpaceS}}, Children: []core.Node{workspaceui.Row(actions...)}},
		}
		if here && w.hostMail() && !dismissed.Get() {
			children = append(children, w.moveNote(cx, func() {
				dismissed.Set(true)
				if err := cx.Storage().SetItem(moveNoteKey, moveNoteDismissed); err != nil {
					w.problem.Set("Could not remember that the note was dismissed: " + err.Error())
				}
			}))
		}
		return core.Scroll{Style: ui.FillRest, Children: []core.Node{core.View{TestID: "welcome", Label: "Welcome",
			Style:    core.Style{MinHeight: core.Pct(100), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: workspaceui.SpaceS, Padding: core.All(workspaceui.SpaceXL)},
			Children: children}}}
	})
}

// moveNote says how to move a host install's mail into the Flatpak. It
// changes nothing: the move is the 3 commands, run by the user.
func (w *workspace) moveNote(cx *core.Scope, dismiss func()) core.Node {
	kit := workspaceui.ColorsOf(cx)
	caption := func(text string) core.Node { return core.Text{Content: text, Style: ui.TypeCaption} }
	children := []core.Node{caption(moveFound)}
	for i, step := range moveSteps {
		children = append(children, core.View{Style: core.Style{Direction: core.Row, Gap: workspaceui.SpaceXS}, Children: []core.Node{
			caption(string(rune('1'+i)) + "."),
			core.Text{Content: step, Selectable: core.SelectableOn, Style: ui.TypeCode.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)})}}})
	}
	children = append(children, caption(moveSecrets),
		core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{ui.Button{Text: "Dismiss", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: dismiss}}})
	return core.View{TestID: "move-note", Role: core.RoleStatus, Label: "Mail from another install",
		Style:     core.Style{Width: core.Pct(100), MaxWidth: core.Pt(520), Margin: core.Edges{Top: workspaceui.SpaceL}, Padding: core.All(workspaceui.SpaceM), Gap: workspaceui.SpaceS, CornerRadius: ui.RadiusControl},
		StyleFunc: func() core.Style { return core.Style{Background: kit.Fill(workspaceui.Neutral)} }, Children: children}
}
