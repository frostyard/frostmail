//go:build !js

package main

import (
	"fmt"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/launcher"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"log/slog"
	"os"
	"strconv"
)

func (w *workspace) platformStart() {
	cx := w.cx
	window := cx.CurrentWindow()
	if window == nil {
		return
	}
	configFile, _ := config.AppConfigFile(os.Getenv)
	w.trayChoice.Set(settings.TrayPreference(configFile))
	show := func() { window.Focus() }
	tray := cx.NewTray(core.TrayOptions{Icon: icons.Tray(), Tooltip: "FrostMail", OnClick: show, Menu: func() []core.MenuItem {
		return []core.MenuItem{{Label: "Open FrostMail", OnSelect: show}, {Label: "Compose", OnSelect: func() {
			show()
			if w.overlay.GetUntracked() == nil {
				w.newDraft(api.DraftKindNew, nil)
			}
		}}, {Label: "Quit", OnSelect: func() {
			show()
			if w.overlay.GetUntracked() != nil {
				w.problem.Set("Close the open dialog before quitting.")
				return
			}
			cx.Quit()
		}}}
	}})
	count := func() int64 { return mail.InboxesUnread(w.store.State().Mailboxes) }
	badge := launcher.Open(slog.Default(), count)
	cx.Cleanup(badge.Close)
	cx.Effect(func() {
		w.state.Get()
		n := count()
		badge.Changed()
		cx.SetBadge(int(n))
		if tray != nil {
			tray.SetTooltip(fmt.Sprintf("FrostMail · %d unread", n))
		}
	})
	if tray != nil {
		w.trayAvailable = true
		window.OnClosing(cx, func(event *core.WindowClosing) {
			if w.trayChoice.GetUntracked() && !event.Canceled() && frostui.TrayHosted() {
				event.Cancel()
				window.Hide()
			}
		})
	}
	cx.OnReopen(show)
	for i, arg := range os.Args {
		if (arg == "--reply-message" || arg == "--open-message") && i+1 < len(os.Args) {
			id, err := strconv.ParseInt(os.Args[i+1], 10, 64)
			if err == nil && id > 0 {
				w.whenConnected(func() {
					if arg == "--reply-message" {
						w.newDraft(api.DraftKindReply, &id)
					} else {
						w.openMessage(id, false)
					}
				})
				break
			}
		}
	}
}
func (w *workspace) traySetting() core.Node {
	if !w.trayAvailable {
		return nil
	}
	return ui.Checkbox{Text: "Close to tray", Checked: w.trayChoice.Get, OnChange: func(on bool) {
		path, err := config.AppConfigFile(os.Getenv)
		if err == nil {
			err = settings.WriteTrayPreference(path, on)
		}
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.trayChoice.Set(on)
	}}
}
