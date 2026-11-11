package pim

import (
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/commands"
)

// On a change of maild the module forgets the last one's selections and
// undo entries: the same id means another item on the next maild.
func TestLeaveForgetsTheLastMaild(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.m.tasks.selected = f.report
	f.m.people.selected = 7
	f.m.people.undo = []string{"token"}
	f.m.cal.undo = []string{"token"}
	f.m.cal.selected = &occKey{1, ""}
	f.m.setTaskSource(sourceAll)
	conn := f.m.conn
	f.m.Leave()
	if f.m.tasks.selected != 0 || f.m.people.selected != 0 || f.m.cal.selected != nil {
		t.Errorf("selections kept: task %d, person %d, occurrence %v", f.m.tasks.selected, f.m.people.selected, f.m.cal.selected)
	}
	if len(f.m.people.undo) != 0 || len(f.m.cal.undo) != 0 {
		t.Errorf("undo kept: people %v, calendar %v", f.m.people.undo, f.m.cal.undo)
	}
	if f.m.tasks.source != sourceToday || f.m.conn == conn {
		t.Errorf("source %v, conn %d (was %d)", f.m.tasks.source, f.m.conn, conn)
	}
}
