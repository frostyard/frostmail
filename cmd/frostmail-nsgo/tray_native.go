//go:build !js

package main

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/launcher"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/desktop/trayhost"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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
		hosts := trayhost.New()
		window.OnClosing(cx, func(event *core.WindowClosing) {
			if w.trayChoice.GetUntracked() && !event.Canceled() && hosts.Hosted() {
				event.Cancel()
				window.Hide()
			}
		})
	}
	cx.OnReopen(show)
	cx.OnOpenLinks(func(links []string) {
		for _, raw := range links {
			link, err := url.Parse(raw)
			if err != nil || link.Scheme != "frostmail-view" || link.User != nil || link.Host != w.notificationKey() {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(link.Path, "/"), "/")
			if len(parts) != 2 || (parts[0] != "open" && parts[0] != "reply") {
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			show()
			w.whenConnected(func() {
				if link.Host != w.notificationKey() {
					return
				}
				open := func() {
					w.closePanel()
					w.module.Set("Mail")
					if parts[0] == "reply" {
						w.newDraft(api.DraftKindReply, &id)
					} else {
						w.openMessage(id, false)
					}
				}
				if save := w.beforeRoute; save != nil {
					save(open, func() {})
				} else {
					open()
				}
			})
			break
		}
	})
}

// NSGo forwards links from a second invocation to the existing window. Map
// the established launcher arguments to a link scoped to this maild target.
func prepareLaunch() {
	for i, arg := range os.Args[1:] {
		if arg != "--open-message" && arg != "--reply-message" || i+2 >= len(os.Args) {
			continue
		}
		id, err := strconv.ParseInt(os.Args[i+2], 10, 64)
		if err != nil || id <= 0 {
			return
		}
		target, err := platformTarget()
		if err != nil {
			return
		}
		verb := "open"
		if arg == "--reply-message" {
			verb = "reply"
		}
		link := fmt.Sprintf("frostmail-view://%x/%s/%d", sha256.Sum256([]byte(target.Key())), verb, id)
		os.Args = append([]string{os.Args[0]}, link)
		return
	}
}

func (w *workspace) traySetting() core.Node {
	if !w.trayAvailable {
		return nil
	}
	return ui.Switch{Label: "Close to tray", Checked: w.trayChoice.Get, OnChange: func(on bool) {
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
