package commands

import "github.com/frostyard/frostui"

// Module is one of the main window's four modules (docs/specs/pim-ui.md,
// Modules; ADR-0020).
type Module string

// The modules, in the module bar's and Ctrl+1–4's order.
const (
	Mail     Module = "mail"
	Calendar Module = "calendar"
	People   Module = "people"
	Tasks    Module = "tasks"
)

// Modules lists the modules in order.
var Modules = []Module{Mail, Calendar, People, Tasks}

// Label names a module.
func (m Module) Label() string {
	switch m {
	case Calendar:
		return "Calendar"
	case People:
		return "People"
	case Tasks:
		return "Tasks"
	}
	return "Mail"
}

// Need is what a command needs before it can run.
type Need int

// What commands need.
const (
	NeedNothing   Need = iota
	NeedSelection      // anything selected in the module
	NeedMessage        // exactly one message shown
)

// Entry names a command in the modules where it acts, puts it in a group
// of the palette, and says what it needs.
type Entry struct {
	Command Command
	Names   map[Module]string
	Group   string
	Needs   Need
}

// The palette's groups.
const (
	GroupMessage = "Message"
	GroupEdit    = "Edit"
	GroupGoTo    = "Go to"
	GroupView    = "View"
	GroupApp     = "App"
)

func each(name string) map[Module]string {
	return map[Module]string{Mail: name, Calendar: name, People: name, Tasks: name}
}
func mail(name string) map[Module]string     { return map[Module]string{Mail: name} }
func calendar(name string) map[Module]string { return map[Module]string{Calendar: name} }

// Entries are the palette's commands in the order it lists them: PALETTE
// of app/src/lib/keymap.ts and the desktop's own. A move (Next, Escape,
// Open) has none: it has no name of its own to find.
var Entries = []Entry{
	{Compose, map[Module]string{Mail: "New Message", Calendar: "New Event", People: "New Contact", Tasks: "New Task"}, GroupApp, NeedNothing},
	{Reply, mail("Reply"), GroupMessage, NeedSelection},
	{ReplyAll, mail("Reply All"), GroupMessage, NeedSelection},
	{Forward, mail("Forward"), GroupMessage, NeedSelection},
	{Redirect, mail("Redirect…"), GroupMessage, NeedSelection},
	{Archive, mail("Archive"), GroupMessage, NeedSelection},
	{Delete, map[Module]string{Mail: "Delete", Calendar: "Delete", Tasks: "Delete"}, GroupEdit, NeedSelection},
	{Junk, mail("Mark as Spam/Not Spam"), GroupMessage, NeedSelection},
	{ToggleRead, mail("Mark as Read/Unread"), GroupMessage, NeedSelection},
	{ToggleFlag, mail("Flag/Unflag"), GroupMessage, NeedSelection},
	{Undo, map[Module]string{Mail: "Undo", Calendar: "Undo", People: "Undo"}, GroupEdit, NeedNothing},
	{SelectAll, mail("Select All"), GroupEdit, NeedNothing},
	{GetMail, mail("Get Mail"), GroupApp, NeedNothing},
	{FocusSearch, map[Module]string{Mail: "Search", People: "Search"}, GroupApp, NeedNothing},
	{Find, mail("Find in Message"), GroupMessage, NeedMessage},
	{Print, mail("Print…"), GroupMessage, NeedMessage},
	{RawSource, mail("Raw Source…"), GroupMessage, NeedMessage},
	{AllHeaders, mail("Show/Hide All Headers"), GroupMessage, NeedMessage},
	{Today, calendar("Go to Today"), GroupGoTo, NeedNothing},
	{DayView, calendar("Day View"), GroupView, NeedNothing},
	{WeekView, calendar("Week View"), GroupView, NeedNothing},
	{MonthView, calendar("Month View"), GroupView, NeedNothing},
	{PreviousPeriod, calendar("Previous Period"), GroupGoTo, NeedNothing},
	{NextPeriod, calendar("Next Period"), GroupGoTo, NeedNothing},
	{AllInboxes, each("All Inboxes"), GroupGoTo, NeedNothing},
	{ShowCalendar, each("Show Calendar"), GroupGoTo, NeedNothing},
	{ShowPeople, each("Show People"), GroupGoTo, NeedNothing},
	{ShowTasks, each("Show Tasks"), GroupGoTo, NeedNothing},
	{ToggleSidebar, each("Show/Hide Sidebar"), GroupView, NeedNothing},
	{Settings, each("Settings…"), GroupApp, NeedNothing},
	{Shortcuts, each("Keyboard Shortcuts"), GroupApp, NeedNothing},
	{About, each("About Frostmail"), GroupApp, NeedNothing},
}

// selectionReason says what a module's selection is, for a command that
// needs one (docs/specs/ui.md, Command palette: Disabled rows).
var selectionReason = map[Module]string{
	Mail:     "Select a message first",
	Calendar: "Select an event first",
	People:   "Select a contact first",
	Tasks:    "Select a task first",
}

const messageReason = "Select one message first"

// moves are the commands without a name to find: they have keys, and the
// palette does not list them.
var moves = []Command{Left, Right, Previous, Next, ExtendPrevious, ExtendNext, First, Last, PageDown, PageUp,
	Escape, NextPane, PreviousPane, ContextMenu, Open}

// Table lists a module's commands that the window can run: those of
// Entries with a name in the module and a function in run, in Entries'
// order, then the moves, unlisted. selected reads the number of rows
// selected, for the commands that need some.
func Table(m Module, selected func() int, run map[Command]func()) []frostui.Command {
	var cmds []frostui.Command
	for _, e := range Entries {
		name, ok := e.Names[m]
		fn := run[e.Command]
		if !ok || fn == nil {
			continue
		}
		cmds = append(cmds, frostui.Command{ID: string(e.Command), Title: name, Group: e.Group, Run: fn,
			Bindings: Keys(e.Command), Enabled: enabled(m, e.Needs, selected)})
	}
	for _, c := range moves {
		if fn := run[c]; fn != nil {
			cmds = append(cmds, frostui.Command{ID: string(c), Title: string(c), Group: GroupApp, Run: fn, Bindings: Keys(c), Unlisted: true})
		}
	}
	return cmds
}

// enabled is the rule of a command that needs a selection, nil for one
// that needs none.
func enabled(m Module, need Need, selected func() int) func() (bool, string) {
	switch need {
	case NeedSelection:
		return func() (bool, string) { return selected() > 0, selectionReason[m] }
	case NeedMessage:
		return func() (bool, string) { return selected() == 1, messageReason }
	}
	return nil
}
