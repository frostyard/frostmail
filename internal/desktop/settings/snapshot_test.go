package settings_test

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// TestPanesAtTheWindowsSizes draws each pane at the window's size and its
// smallest, and writes pictures when FROSTMAIL_SNAPSHOT_DIR asks for them.
func TestPanesAtTheWindowsSizes(t *testing.T) {
	f := newFixture(t, setup{first: func(f *fixture) {
		a := f.addIMAP("me@a.test")
		name := "Ann Example"
		if _, err := f.c.Account().Update(t.Context(), &api.AccountUpdateParams{ID: a.ID, DisplayName: &name}); err != nil {
			t.Fatal(err)
		}
		f.addRule("Receipts")
		if _, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Hello", HTML: "<p>Hi</p>"}); err != nil {
			t.Fatal(err)
		}
	}})
	f.sees("Ann Example")
	if _, ok := f.tt.AccessByRole(ui.RoleList, "Accounts"); !ok {
		t.Errorf("no list of accounts in %q", f.tt.Texts())
	}
	for _, size := range [][2]int{{settings.Width, settings.Height}, {settings.MinWidth, settings.MinHeight}} {
		f.tt.SetSize(size[0], size[1])
		for _, pane := range []string{settings.PaneGeneral, settings.PaneAccounts, settings.PaneSignatures, settings.PaneSnippets, settings.PaneRules, settings.PaneSignIn, settings.PaneServer} {
			f.show(pane)
			f.tt.Frame()
			writeSnapshot(t, f.tt, pane, size[0])
		}
	}
}

func writeSnapshot(t *testing.T, tt *ui.Tester, pane string, width int) {
	t.Helper()
	dir := os.Getenv("FROSTMAIL_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	out, err := os.Create(filepath.Join(dir, pane+"-"+string(rune('0'+width/100))+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
