package settings

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
)

// ServerFacts are the Server pane's local facts: no request to maild, and
// none to lodge, so the pane says the same while either is unreachable.
type ServerFacts struct {
	// Flatpak is true in this app's Flatpak: a change of mode stops or
	// starts its maild.
	Flatpak bool
	// LocalData is true when maild's data directory holds a database.
	LocalData bool
	// HostMail is true when a Frostmail installed outside Flatpak has mail
	// here, which the move note says how to bring in.
	HostMail bool
}

// FactsFrom reads the facts from the environment, as the shell did. Only
// the files' metadata is read: the database belongs to maild.
func FactsFrom(getenv func(string) string) ServerFacts {
	f := ServerFacts{Flatpak: config.InFlatpak(getenv)}
	if p, err := config.Resolve(func(key string) string {
		// The socket is not asked for: a missing runtime directory must
		// not hide the data directory.
		if key == "FROSTMAIL_SOCKET" {
			return "unused"
		}
		// A relative data directory names one in maild's own working
		// directory, which the app cannot know.
		if v := getenv(key); key != "FROSTMAIL_DATA_DIR" || filepath.IsAbs(v) {
			return v
		}
		return ""
	}); err == nil {
		f.LocalData = fileExists(p.DB)
	}
	// In the Flatpak XDG_DATA_HOME is the app's own, so only HOME says
	// where a host install keeps its mail; the development bundle sets
	// FROSTMAIL_SOCKET, because its maild is the host's.
	if home := getenv("HOME"); f.Flatpak && getenv("FROSTMAIL_SOCKET") == "" && filepath.IsAbs(home) {
		f.HostMail = fileExists(filepath.Join(home, ".local", "share", "frostmail", "frostmail.db"))
	}
	return f
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// serverPane is the Server pane: which maild the app asks. It stays across
// connections, because a change of mode ends the one that started it.
type serverPane struct {
	stored  string // lodge's address, or "" for this computer's maild
	remote  bool
	address string
	busy    bool
	status  string
	err     string
	saved   int // finished saves: each makes the form anew on the stored choice
}

// The words of a change of mode (docs/specs/settings-ui.md, Server pane).
const (
	noteToLodge     = "This computer's maild stops. Its accounts and mail stay on this computer, not synced, until you switch back. Nothing moves to lodge."
	noteToLocalData = "This computer's maild starts again with the accounts it had. It runs their rules, Send Later and reminders here. If lodge's maild has the same accounts, both run them."
	noteToLocal     = "Mail on lodge keeps running there. If you add the same accounts here, both run their rules, Send Later and reminders."
	statusStopping  = "Stopping this computer's maild…"
	statusStarting  = "Starting this computer's maild…"
)

// saveStatus is the status a save shows while it runs, fixed when it starts.
func saveStatus(flatpak bool, from, to string) string {
	switch {
	case !flatpak:
		return ""
	case from == "" && to != "":
		return statusStopping
	case from != "" && to == "":
		return statusStarting
	}
	return ""
}

// readServer reads the stored choice afresh. kept keeps a finished save's
// error, which says more than the read after it.
func (w *Window) readServer(kept bool) {
	s := &w.server
	cfg, err := conn.ReadAppConfig(w.o.ConfigFile)
	if err != nil {
		if !kept || s.err == "" {
			s.err = err.Error()
		}
		return
	}
	s.stored, s.remote, s.address = cfg.Lodge, cfg.Lodge != "", cfg.Lodge
}

// loadFacts reads the local facts afresh, when the app can.
func (w *Window) loadFacts() {
	if w.o.LoadFacts != nil {
		w.facts = w.o.LoadFacts()
	}
}

// serverNote says what saving the chosen mode does, from local facts
// alone, so it says the same while lodge is unreachable.
func (w *Window) serverNote() string {
	s := &w.server
	switch {
	case !w.facts.Flatpak:
		return ""
	case s.remote && s.stored == "":
		return noteToLodge
	case !s.remote && s.stored != "":
		if w.facts.LocalData {
			return noteToLocalData
		}
		return noteToLocal
	}
	return ""
}

func (w *Window) serverBody(c *ui.Context) {
	s := &w.server
	// Read when asked: the controls below take their input as they are
	// drawn, so s does not hold this frame's yet.
	chosen := func() string {
		if s.remote {
			return strings.TrimSpace(s.address)
		}
		return ""
	}
	off := func() bool { return s.busy || s.remote && chosen() == "" || chosen() == s.stored }
	save := func() {
		if !off() {
			w.saveServer(chosen())
		}
	}
	ui.Column(c.Key(s.saved)).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label("Server").Children(func() {
		note(c, "Frostmail reads your mail through maild: the one on this computer, or the one behind lodge on your tailnet, which your phone and browsers use too.")
		ui.RadioGroup(c, func() {
			ui.Radio(c, &s.remote, false, "maild on this computer")
			ui.Radio(c, &s.remote, true, "maild behind lodge")
		}).Label("Server")
		if s.remote {
			ui.Form(c, func() {
				ui.Field(c.Key("lodge"), "lodge:", func() {
					if ui.TextInput(c, &s.address).Placeholder("https://lodge.example.ts.net").Submitted() {
						save()
					}
				})
			})
		}
		if text := w.serverNote(); text != "" {
			note(c, text)
		}
		if s.busy && s.status != "" {
			note(c, s.status).Role(ui.RoleStatus)
		}
		alert(c, s.err)
		buttons(c, func() {
			if ui.PrimaryButton(c, "Save").Disabled(off()).Clicked() {
				save()
			}
		})
	})
}
