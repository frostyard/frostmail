package mainwin

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A redirect on its way goes on: while it is, nothing offers to cancel it.
func TestRedirectSheetWaitsForTheSend(t *testing.T) {
	w := &Window{}
	s := &redirectSheet{to: "bo@x.test", busy: true}
	w.redirect = s
	tt := ui.NewTester(w.redirectView, 800, 600)
	button := func(name string) ui.AccessInfo {
		t.Helper()
		info, ok := tt.AccessByRole(ui.RoleButton, name)
		if !ok {
			t.Fatalf("no %s in %q", name, tt.Texts())
		}
		return info
	}
	for _, name := range []string{"Cancel", "Redirect"} {
		if !button(name).Disabled {
			t.Errorf("%s is enabled while the redirect is on its way", name)
		}
	}
	cancel := button("Cancel")
	tt.ClickAt(cancel.Bounds.X+cancel.Bounds.W/2, cancel.Bounds.Y+cancel.Bounds.H/2)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if w.redirect != s {
		t.Fatal("the sheet closed while the redirect was on its way")
	}

	s.busy = false
	tt.Frame()
	if button("Cancel").Disabled {
		t.Error("Cancel is off with nothing on its way")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.redirect != nil {
		t.Error("Cancel left the sheet open")
	}
}
