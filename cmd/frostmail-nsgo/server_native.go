//go:build !js

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
)

const desktopServer = true

func (w *workspace) serverPanel(cx *core.Scope) core.Node {
	path, err := config.AppConfigFile(os.Getenv)
	if err != nil {
		return core.Text{Content: err.Error()}
	}
	cfg, err := conn.ReadAppConfig(path)
	if err != nil {
		return core.Text{Content: err.Error()}
	}
	value := cfg.Lodge
	facts := settings.FactsFrom(os.Getenv)
	nodes := []core.Node{core.Text{Content: "Use the local mail daemon or the mail account store behind Lodge. Each server keeps its own mail and drafts."}, stringField("Lodge address (empty for local maild)", &value, false)}
	if facts.Flatpak {
		nodes = append(nodes, core.Text{Content: "Switching to Lodge stops this computer’s maild. Switching back starts its sync, rules, and outbox again."})
	}
	nodes = append(nodes, button("Save server", func() {
		next := strings.TrimSpace(value)
		if next == cfg.Lodge {
			return
		}
		if next != "" {
			if _, err := conn.ParseLodge(next); err != nil {
				w.problem.Set(err.Error())
				return
			}
		}
		w.switching.Set(true)
		go func() {
			command := func(ctx context.Context, name string, arg string) error {
				cmd := exec.CommandContext(ctx, name, arg)
				cmd.WaitDelay = time.Second
				out, err := cmd.CombinedOutput()
				if err != nil && len(out) > 0 {
					return errors.New(strings.TrimSpace(string(out)))
				}
				return err
			}
			moved, err := settings.ChangeServer(settings.Options{ConfigFile: path, StopLocal: func(ctx context.Context) error { return command(ctx, "mailctl", "shutdown") }, StartLocal: func(ctx context.Context) error { return command(ctx, "frostmail", "--start-maild") }}, facts.Flatpak, conn.AppConfig{Lodge: next})
			target, targetErr := platformTarget()
			w.cx.Post(func() {
				if moved && targetErr == nil && !w.session.Target().Same(target) {
					w.closePanel()
					w.store.Disconnect()
					w.store.Forget()
					w.toaster.DismissAll()
					w.undo = &mail.Undo{}
					w.source = mail.Source{Kind: mail.AllInboxes}
					w.selected.Set(w.source.Key())
					w.term.Set("")
					w.search = ""
					w.clearView()
					w.pendingTarget = target.Key()
					w.session.SetTarget(target)
				}
				if w.pendingTarget == "" {
					w.switching.Set(false)
				}
				if err != nil {
					w.problem.Set(err.Error())
				} else if targetErr != nil {
					w.problem.Set(targetErr.Error())
				} else {
					w.problem.Set("")
				}
			})
		}()
	}), button("Retry connection", w.session.Retry))
	return workspaceui.Page(nodes...)
}
