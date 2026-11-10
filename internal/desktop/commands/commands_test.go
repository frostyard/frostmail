package commands_test

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/desktop/commands"
)

type chord struct {
	mods ui.Modifiers
	key  ui.Key
}

// The cases of keymap.test.ts (T-0034's contract), and the desktop's keys.
func TestForOutsideTextFields(t *testing.T) {
	for k, want := range map[chord]commands.Command{
		{0, ui.KeyUp}: commands.Previous, {0, ui.KeyDown}: commands.Next, {ui.Shift, ui.KeyUp}: commands.ExtendPrevious,
		{ui.Shift, ui.KeyDown}: commands.ExtendNext, {0, ui.KeyHome}: commands.First, {0, ui.KeyEnd}: commands.Last,
		{ui.Cmd, ui.KeyA}: commands.SelectAll, {0, ui.KeySpace}: commands.PageDown,
		{ui.Shift, ui.KeySpace}: commands.PageUp, {0, ui.KeyDelete}: commands.Delete, {0, ui.KeyBackspace}: commands.Delete,
		{ui.Cmd | ui.Alt, ui.KeyA}: commands.Archive, {ui.Cmd | ui.Shift, ui.KeyU}: commands.ToggleRead,
		{ui.Cmd | ui.Shift, ui.KeyL}: commands.ToggleFlag, {ui.Cmd | ui.Alt, ui.KeyF}: commands.FocusSearch, {ui.Cmd, ui.KeyF}: commands.Find,
		{0, ui.KeyEscape}: commands.Escape, {ui.Cmd | ui.Shift, ui.KeyN}: commands.GetMail, {ui.Cmd, ui.Key1}: commands.AllInboxes,
		{ui.Cmd, ui.KeyComma}: commands.Settings, {ui.Cmd | ui.Alt, ui.KeyS}: commands.ToggleSidebar, {0, ui.KeyTab}: commands.NextPane,
		{ui.Shift, ui.KeyTab}: commands.PreviousPane, {0, ui.KeyContextMenu}: commands.ContextMenu, {ui.Shift, ui.KeyF10}: commands.ContextMenu,
		{ui.Cmd, ui.KeyN}: commands.Compose, {ui.Cmd, ui.KeyR}: commands.Reply, {ui.Cmd | ui.Shift, ui.KeyR}: commands.ReplyAll,
		{ui.Cmd | ui.Shift, ui.KeyF}: commands.Forward, {0, ui.KeyEnter}: commands.Open,
		{ui.Cmd | ui.Shift, ui.KeyJ}: commands.Junk,
	} {
		if got, ok := commands.For(k.mods, k.key, false); !ok || got != want {
			t.Errorf("%v = %q, %v; want %s", k, got, ok, want)
		}
	}
	for _, k := range []chord{{0, ui.KeyA}, {ui.Cmd | ui.Shift, ui.KeyA}, {ui.Alt, ui.KeyUp}, {ui.Cmd, ui.KeyDown}, {ui.Cmd, ui.KeyDelete},
		{ui.Shift, ui.KeyDelete}, {ui.Cmd | ui.Alt | ui.Shift, ui.KeyA}, {ui.Cmd, ui.Key5}, {ui.Cmd | ui.Alt, ui.KeyN},
		{ui.Cmd | ui.Alt, ui.KeyR}, {0, ui.KeyF10}, {ui.Alt, ui.KeyTab}} {
		if got, ok := commands.For(k.mods, k.key, false); ok {
			t.Errorf("%v = %s, want nothing", k, got)
		}
	}
}

func TestForInATextField(t *testing.T) {
	for k, want := range map[chord]commands.Command{
		{0, ui.KeyEscape}: commands.Escape, {0, ui.KeyTab}: commands.NextPane, {ui.Shift, ui.KeyTab}: commands.PreviousPane,
		{ui.Cmd | ui.Shift, ui.KeyN}: commands.GetMail, {ui.Cmd, ui.Key1}: commands.AllInboxes, {ui.Cmd, ui.KeyComma}: commands.Settings,
		{ui.Cmd | ui.Alt, ui.KeyF}: commands.FocusSearch, {ui.Cmd | ui.Alt, ui.KeyS}: commands.ToggleSidebar, {ui.Cmd | ui.Shift, ui.KeyU}: commands.ToggleRead,
		{ui.Cmd | ui.Shift, ui.KeyF}: commands.Forward,
	} {
		if got, ok := commands.For(k.mods, k.key, true); !ok || got != want {
			t.Errorf("%v = %q, %v; want %s", k, got, ok, want)
		}
	}
	for _, k := range []chord{{0, ui.KeyUp}, {0, ui.KeyDown}, {0, ui.KeySpace}, {0, ui.KeyDelete}, {0, ui.KeyBackspace},
		{ui.Cmd, ui.KeyA}, {ui.Cmd, ui.KeyF}, {0, ui.KeyEnter}} {
		if got, ok := commands.For(k.mods, k.key, true); ok {
			t.Errorf("%v in a field = %s, want nothing", k, got)
		}
	}
}

func TestEveryKeyIsBoundOnce(t *testing.T) {
	seen := map[chord]commands.Command{}
	for _, b := range commands.Bindings {
		k := chord{b.Mods, b.Key}
		if prev, ok := seen[k]; ok {
			t.Errorf("%v binds both %s and %s", k, prev, b.Command)
		}
		seen[k] = b.Command
	}
}

func all() map[commands.Command]func() {
	run := map[commands.Command]func(){}
	for _, e := range commands.Entries {
		run[e.Command] = func() {}
	}
	return run
}

func ids(cmds []frostui.Command) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, c.ID)
	}
	return out
}

func TestTableListsOnlyTheModulesCommands(t *testing.T) {
	cal := ids(commands.Table(commands.Calendar, func() int { return 0 }, all()))
	if slices.Contains(cal, string(commands.Archive)) {
		t.Errorf("Calendar lists Archive: %q", cal)
	}
	if !slices.Contains(cal, string(commands.DayView)) {
		t.Errorf("Calendar lacks Day View: %q", cal)
	}
	mail := commands.Table(commands.Mail, func() int { return 0 }, all())
	if slices.Contains(ids(mail), string(commands.DayView)) {
		t.Errorf("Mail lists Day View: %q", ids(mail))
	}
	if i := slices.IndexFunc(mail, func(c frostui.Command) bool { return c.ID == string(commands.Compose) }); i < 0 || mail[i].Title != "New Message" {
		t.Errorf("Mail's Compose is %+v", mail[i])
	}
}

func TestTableOmitsWhatTheWindowCannotRun(t *testing.T) {
	cmds := commands.Table(commands.Mail, func() int { return 0 }, map[commands.Command]func(){commands.Archive: func() {}})
	if got := ids(cmds); !slices.Equal(got, []string{string(commands.Archive)}) {
		t.Errorf("table = %q", got)
	}
}

func TestReasons(t *testing.T) {
	n := 0
	table := frostui.NewCommandTable(commands.Table(commands.Mail, func() int { return n }, all())...)
	if ok, why := table.Enabled(string(commands.Archive)); ok || why != "Select a message first" {
		t.Errorf("Archive with none selected: %v, %q", ok, why)
	}
	n = 2
	if ok, why := table.Enabled(string(commands.Print)); ok || why != "Select one message first" {
		t.Errorf("Print with two selected: %v, %q", ok, why)
	}
	if ok, _ := table.Enabled(string(commands.Archive)); !ok {
		t.Error("Archive with a selection is off")
	}
	n = 1
	if ok, _ := table.Enabled(string(commands.Print)); !ok {
		t.Error("Print with one selected is off")
	}
}

func TestMovesAreUnlisted(t *testing.T) {
	run := all()
	run[commands.Next] = func() {}
	for _, c := range commands.Table(commands.Mail, func() int { return 0 }, run) {
		if c.ID == string(commands.Next) && !c.Unlisted {
			t.Error("Next is listed")
		}
	}
}
