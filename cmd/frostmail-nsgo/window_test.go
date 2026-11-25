//go:build linux && integration

package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// windowKey is where the kit keeps the window (workspaceui.KeepWindow),
// and keptWindow what it keeps there.
const windowKey = "window"

type keptWindow struct{ Width, Height core.Points }

// keptIn reads a kept value from the app's storage.
func keptIn[T any](a *nsgotest.App, key string) (v T, ok bool) {
	text, ok := a.Storage().GetItem(key)
	return v, ok && json.Unmarshal([]byte(text), &v) == nil
}

// TestWindowAndViewAreKept checks that a change of the window's size, of
// a pane's width and of how the list is shown is kept on the device, and
// that a window under the least size is not.
func TestWindowAndViewAreKept(t *testing.T) {
	a := fixtureWorkspace(t)
	a.WaitFor("id:messages >> label:3 messages", 8*time.Second)
	if _, kept := a.Storage().GetItem(windowKey); kept {
		t.Fatal("the window's first size is kept before it changed")
	}
	a.Resize(1100, 760)
	a.Advance(time.Second)
	a.WaitUntil(5*time.Second, func() bool {
		k, ok := keptIn[keptWindow](a, windowKey)
		return ok && k.Width == 1100 && k.Height == 760
	})
	a.Resize(300, 200)
	a.Advance(time.Second)
	if k, _ := keptIn[keptWindow](a, windowKey); k.Width != 1100 || k.Height != 760 {
		t.Errorf("a window 300 by 200 replaced the kept size: %+v", k)
	}
	a.Resize(1280, 820)

	// The list's width, and then the navigation's, which leaves the list's.
	a.Drag("label:Resize message list", core.Point{X: 50})
	a.Advance(time.Second)
	a.WaitUntil(5*time.Second, func() bool { k, ok := keptIn[keptPanes](a, panesKey); return ok && k.List > listFirst })
	list, _ := keptIn[keptPanes](a, panesKey)
	a.Drag("label:Resize navigation", core.Point{X: 30})
	a.Advance(time.Second)
	a.WaitUntil(5*time.Second, func() bool { k, _ := keptIn[keptPanes](a, panesKey); return k.Navigation > 246 })
	if k, _ := keptIn[keptPanes](a, panesKey); k.List != list.List {
		t.Errorf("keeping the navigation's width changed the list's kept width from %v to %v", list.List, k.List)
	}

	// How the list is shown.
	a.Press("label:Sort")
	a.Press("label:Show conversations")
	a.WaitUntil(5*time.Second, func() bool { k, ok := keptIn[keptView](a, viewKey); return ok && !k.Conversations && k.Photos })
	a.Press("label:Sort")
	a.Press("label:From")
	a.WaitUntil(5*time.Second, func() bool {
		k, _ := keptIn[keptView](a, viewKey)
		return k.Sort == api.ViewSortFrom && k.Ascending
	})
}

// TestKeptWindowShowsAtStart checks that a start shows the window, its
// panes and its list as the device kept them, and that a sort the device
// kept and the client does not know is left out.
func TestKeptWindowShowsAtStart(t *testing.T) {
	a := fixtureWith(t, "", 1280, 820, false, nsgotest.Storage(map[string]string{
		windowKey: `{"Width":1180,"Height":780}`,
		panesKey:  `{"Navigation":300,"Inspector":0,"Auxiliary":0,"List":470}`,
		viewKey:   `{"Photos":true,"Conversations":false,"Sort":"no-such-sort","Ascending":false}`,
	}))
	a.WaitFor("id:messages >> id:summary-when >> nth=0", 8*time.Second)
	// Headless, the host records what the app asked of its window and
	// keeps its size; a real window takes the size.
	if nsgotest.Target() == "" {
		asked := slices.ContainsFunc(a.Window, func(r nsgotest.WindowRequest) bool {
			return r.Action == nsgotest.WindowSetFrame && r.Frame.Width == 1180 && r.Frame.Height == 780
		})
		if !asked {
			t.Errorf("the app did not ask for its kept window, 1180 by 780: %+v", a.Window)
		}
	} else {
		a.WaitUntil(5*time.Second, func() bool {
			size := a.Find("id:frame").Bounds().Size
			return size.Width == 1180 && size.Height == 780
		})
	}
	a.WaitUntil(5*time.Second, func() bool { return a.Find("id:sidebar-panel").Bounds().Size.Width == 300 })
	// The list is 470 wide less its inset of 8 at each side.
	if width := a.Find("id:messages").Bounds().Size.Width; width < 440 || width > 470 {
		t.Errorf("the message list is %v wide, want the kept 470 less its inset", width)
	}
	// With conversations off no row counts a conversation's messages.
	if a.Exists("id:messages >> label:3 messages") {
		t.Error("a row counts its conversation with conversations kept off")
	}
	// The unknown sort is not taken: the list is by date, newest first.
	a.Press("label:Sort")
	a.WaitFor("label:Show conversations", 5*time.Second)
	if k, kept := keptIn[keptView](a, viewKey); !kept || k.Sort != "no-such-sort" {
		t.Errorf("the start rewrote what the device kept: %+v", k)
	}
}

// TestTodoBarIsKept checks that the To-Do Bar opens as the device kept it,
// and that opening and closing it is kept.
func TestTodoBarIsKept(t *testing.T) {
	a := fixtureWith(t, "", 1280, 820, false, nsgotest.Storage(map[string]string{todoKey: `true`}))
	a.WaitFor("label:New task title", 8*time.Second)
	a.Press("label:To-Do Bar")
	a.WaitForGone("label:New task title", 5*time.Second)
	a.WaitUntil(5*time.Second, func() bool { open, ok := keptIn[bool](a, todoKey); return ok && !open })
	a.Press("label:To-Do Bar")
	a.WaitFor("label:New task title", 5*time.Second)
	a.WaitUntil(5*time.Second, func() bool { open, _ := keptIn[bool](a, todoKey); return open })
}

// TestTodoBarStartsShutWithNothingKept checks a first start.
func TestTodoBarStartsShutWithNothingKept(t *testing.T) {
	a := fixtureWorkspace(t)
	a.WaitFor("id:messages >> label:3 messages", 8*time.Second)
	if a.Exists("label:New task title") {
		t.Error("the To-Do Bar is open at a first start")
	}
	if _, kept := a.Storage().GetItem(todoKey); kept {
		t.Error("the To-Do Bar's state is kept before it changed")
	}
}
