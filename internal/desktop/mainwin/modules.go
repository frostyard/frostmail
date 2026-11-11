package mainwin

import (
	"encoding/json/v2"
	"log/slog"
	"maps"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/pim"
)

// moduleIcons are the module bar's icons.
var moduleIcons = map[commands.Module]string{
	commands.Mail: "mail-unread", commands.Calendar: "x-office-calendar",
	commands.People: "system-users", commands.Tasks: "object-select",
}

// setModule shows a module, unless a contact being edited holds the window.
func (w *Window) setModule(m commands.Module) {
	if w.pim.Inert() {
		return
	}
	w.module = m
}

// showMessage shows a message in Mail.
func (w *Window) showMessage(id int64) {
	w.setModule(commands.Mail)
	w.OpenMessage(id)
}

// pimToast shows a notice, or with an error a failure.
func (w *Window) pimToast(title string, err error) {
	if err != nil {
		w.toast(title, err)
		return
	}
	w.o.Post(func() { w.toasts = append(w.toasts, ui.Toast{Title: title}) })
}

func (w *Window) openWindow(spec pim.WindowSpec) {
	if w.o.OpenWindow != nil {
		w.o.OpenWindow(spec)
	}
}

// moduleHandlers are what the keys and the palette run in the module shown.
func (w *Window) moduleHandlers() map[commands.Command]func() {
	if w.module == commands.Mail {
		return w.handlers
	}
	shared := []commands.Command{commands.ToggleSidebar, commands.AllInboxes, commands.ShowCalendar, commands.ShowPeople,
		commands.ShowTasks, commands.Settings, commands.Shortcuts, commands.About}
	out := maps.Clone(w.pim.Handlers(w.module))
	for _, c := range shared {
		if run := w.handlers[c]; run != nil {
			out[c] = run
		}
	}
	return out
}

// syncTable gives the window's table the module's commands: a module
// change starts a new table, as a command of another module must go.
func (w *Window) syncTable() {
	if w.tableModule != w.module {
		w.tableModule = w.module
		w.table = frostui.NewCommandTable()
		w.table.Editing = func(*ui.Context) bool { return false }
	}
	run := w.moduleHandlers()
	if w.pim.Inert() {
		run = maps.Clone(run)
		for _, c := range []commands.Command{commands.AllInboxes, commands.ShowCalendar, commands.ShowPeople, commands.ShowTasks} {
			delete(run, c)
		}
	}
	selected := func() int {
		if w.module == commands.Mail {
			return len(w.selectedIDs())
		}
		return w.pim.Selected(w.module)
	}
	w.table.Add(commands.Table(w.module, selected, run)...)
	if w.module == commands.People {
		// Ctrl+F has no name of its own in People, and still finds.
		if find := run[commands.Find]; find != nil {
			w.table.Add(frostui.Command{ID: string(commands.Find), Title: "Find", Group: commands.GroupApp, Run: find, Bindings: commands.Keys(commands.Find), Unlisted: true})
		}
	}
	if w.module == commands.Mail && w.readerFocused {
		// In the reader Tab walks its rows and buttons.
		w.table.Remove(string(commands.NextPane), string(commands.PreviousPane))
	}
}

// modulePanes are the panes of the module shown, the module bar at the
// foot of the sidebar.
func (w *Window) modulePanes(c *ui.Context) []frostui.Pane {
	if w.module == commands.Mail {
		return w.panes(c)
	}
	panes := w.pim.Panes(c, w.module)
	for i := range panes {
		if panes[i].ID != paneSidebar {
			continue
		}
		content := panes[i].Content
		panes[i].Content = func() {
			ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(frostui.Chrome(c.Theme())).Children(func() {
				ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(content)
				w.moduleBar(c)
			})
		}
	}
	return panes
}

// moduleBar is the row at the sidebar's foot that switches modules.
func (w *Window) moduleBar(c *ui.Context) {
	ui.Row(c).Height(40).Shrink(0).Gap(frostui.SpaceXS).Padding(0, frostui.SpaceS).AlignItems(ui.Center).Justify(ui.Center).Role(ui.RoleToolbar).Label("Modules").Children(func() {
		for _, m := range commands.Modules {
			on := w.module == m
			if frostui.IconButton(c.Key(m), moduleIcons[m], m.Label()).Selected(on).Role(ui.RoleToggleButton).Checked(on).Disabled(w.pim.Inert()).Clicked() {
				w.setModule(m)
			}
		}
	})
}

// readerAndToDo draws the reader, and the To-Do Bar when it shows. The bar
// is inside the reader's pane, not a pane of its own, so that the top bar
// keeps the room of the pane under it.
func (w *Window) readerAndToDo(c *ui.Context) {
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).Basis(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() { w.reader(c) })
		if w.todoOpen {
			t := c.Theme()
			ui.Column(c).Width(280).Shrink(0).AlignItems(ui.Stretch).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Role(ui.RoleRegion).Label("To-Do Bar").Children(func() {
				w.pim.ToDoBar(c)
			})
		}
	})
}

// uiState is what the window keeps between runs: pane widths, what is
// hidden, and the list's view choices (docs/design/app.md).
type uiState struct {
	Layout        frostui.PaneLayout `json:"layout"`
	ConsoleHeight float32            `json:"consoleHeight,omitzero"`
	ToDo          bool               `json:"todo,omitzero"`
	ContactPhotos bool               `json:"contactPhotos,omitzero"`
	Conversations *bool              `json:"conversations,omitzero"`
	Sort          api.ViewSort       `json:"sort,omitzero"`
	Ascending     bool               `json:"ascending,omitzero"`
}

func (w *Window) loadState() {
	if w.o.StateFile == "" {
		return
	}
	data, err := os.ReadFile(w.o.StateFile)
	if err != nil {
		return // none yet
	}
	var st uiState
	if err := json.Unmarshal(data, &st); err != nil {
		slog.Warn("reading the window's state", "err", err)
		return
	}
	w.shell.Panes.Layout, w.shell.ConsoleHeight = st.Layout, st.ConsoleHeight
	w.todoOpen, w.contactPhotos = st.ToDo, st.ContactPhotos
	if st.Conversations != nil {
		w.q.Conversations = *st.Conversations
	}
	if st.Sort != "" {
		w.q.Sort, w.q.Ascending = st.Sort, st.Ascending
	}
}

// SaveState writes the window's layout, as it closes.
func (w *Window) SaveState() {
	if w.o.StateFile == "" {
		return
	}
	conversations := w.q.Conversations
	data, err := json.Marshal(uiState{Layout: w.shell.Panes.Layout, ConsoleHeight: w.shell.ConsoleHeight, ToDo: w.todoOpen,
		ContactPhotos: w.contactPhotos, Conversations: &conversations, Sort: w.q.Sort, Ascending: w.q.Ascending})
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(w.o.StateFile), 0o700); err == nil {
			err = os.WriteFile(w.o.StateFile, data, 0o600)
		}
	}
	if err != nil {
		slog.Warn("saving the window's state", "err", err)
	}
}
