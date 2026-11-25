//go:build linux && integration

package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// TestEveryDialogHasAWayOut checks the quiet first button of a dialog:
// Cancel before a button that confirms or destroys, Close where there is
// none, and no second one where the dialog names its own.
func TestEveryDialogHasAWayOut(t *testing.T) {
	leave := func() {}
	texts := func(nodes []core.Node) (out []string) {
		for _, n := range nodes {
			if b, ok := n.(ui.Button); ok {
				out = append(out, b.Text)
			} else {
				out = append(out, "(not a button)")
			}
		}
		return out
	}
	for _, tc := range []struct {
		name   string
		footer []core.Node
		want   []string
	}{
		{"no button", nil, []string{"Close"}},
		{"a button that confirms", []core.Node{button("Manual setup", leave), primary("Find settings", leave)}, []string{"Cancel", "Manual setup", "Find settings"}},
		{"a button that destroys", []core.Node{danger("Remove", leave)}, []string{"Cancel", "Remove"}},
		{"quiet buttons alone", []core.Node{button("Copy", leave)}, []string{"Close", "Copy"}},
		{"its own Cancel", []core.Node{button("Cancel", leave), primary("Save", leave)}, []string{"Cancel", "Save"}},
		{"its own Close", []core.Node{button("Close", leave)}, []string{"Close"}},
		{"a node that is no button", []core.Node{core.Text{Content: "note"}, primary("Save", leave)}, []string{"Cancel", "(not a button)", "Save"}},
	} {
		got := texts(withWayOut(tc.footer, leave))
		if len(got) != len(tc.want) {
			t.Errorf("%s: buttons %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: buttons %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
	if first := withWayOut(nil, leave)[0].(ui.Button); first.Variant != ui.ButtonSecondary {
		t.Errorf("the way out is variant %v, want the quiet one", first.Variant)
	}
}
