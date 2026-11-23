package mainwin

import (
	"slices"
	"testing"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestShortcutsListThePaletteFirst(t *testing.T) {
	srv := rpctest.Start(t)
	w, tt := windowOn(t, conn.Target{Socket: srv.Socket})
	draw(t, tt, "the commands", func() bool { return len(w.table.Shortcuts()) > 0 })
	tt.Frame()
	tt.Frame()
	got := w.table.Shortcuts()
	if len(got) < 2 {
		t.Fatalf("got %d shortcuts; want at least two", len(got))
	}
	want := []frostui.ShortcutEntry{
		{Group: commands.GroupApp, Title: "Command Palette", Keys: []string{"Ctrl", "K"}},
		{Group: commands.GroupApp, Title: "Command Palette", Keys: []string{"Ctrl", "Shift", "P"}},
	}
	for i, entry := range want {
		if got[i].Group != entry.Group || got[i].Title != entry.Title || !slices.Equal(got[i].Keys, entry.Keys) {
			t.Errorf("shortcut %d = %+v; want %+v", i, got[i], entry)
		}
	}
	for i, entry := range got[2:] {
		if entry.Title == "Command Palette" {
			t.Errorf("extra Command Palette entry at %d: %+v", i+2, entry)
		}
	}
}
