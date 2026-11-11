//go:build e2e

package e2e

import (
	"testing"

	"golang.org/x/sys/windows"
)

var (
	user32       = windows.NewLazySystemDLL("user32.dll")
	setCursorPos = user32.NewProc("SetCursorPos")
	mouseEvent   = user32.NewProc("mouse_event")
)

const (
	mouseLeftDown = 0x0002
	mouseLeftUp   = 0x0004
)

// realClick is a click the system delivers, which reaches a page: the
// automation plugin's input goes to native UI only.
func realClick(t *testing.T, x, y int) {
	t.Helper()
	if r, _, err := setCursorPos.Call(uintptr(x), uintptr(y)); r == 0 {
		t.Fatalf("SetCursorPos: %v", err)
	}
	_, _, _ = mouseEvent.Call(mouseLeftDown, 0, 0, 0, 0)
	_, _, _ = mouseEvent.Call(mouseLeftUp, 0, 0, 0, 0)
}
