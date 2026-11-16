package main

import (
	"fmt"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/frostyard/frostui"
)

// newMessageID is the tray menu's own item, between Open and Quit.
const newMessageID = "frostmail.new-message"

// trayStatus is the tray menu's first line: the All Inboxes count, which
// the icon's dot stands for.
func trayStatus(n int64) string {
	return fmt.Sprintf("%d unread", n)
}

// trayItems are the app's entries of the tray's menu.
func trayItems(newMessage func()) []frostui.TrayItem {
	return []frostui.TrayItem{{ID: newMessageID, Label: "New Message", Click: newMessage}}
}

// trayIcon is what the tray offers of its status: frostui's Tray
// satisfies it, and a test gives a fake.
type trayIcon interface {
	SetStatus(string)
	SetDot(bool)
}

// trayCount keeps the tray's status line and dot on the All Inboxes
// count. Changed runs on any goroutine and never blocks: it posts one
// update at a time, on the UI thread, where icon and its calls live.
type trayCount struct {
	post    func(func())
	count   func() int64
	pending atomic.Bool
	icon    trayIcon
}

// Changed asks for a new status line; calls that come before the posted
// update runs share its one update.
func (t *trayCount) Changed() {
	if !t.pending.CompareAndSwap(false, true) {
		return
	}
	t.post(func() {
		t.pending.Store(false)
		if t.icon == nil {
			return
		}
		n := t.count()
		t.icon.SetStatus(trayStatus(n))
		t.icon.SetDot(n > 0)
	})
}

// attach gives the counter its icon, on the UI thread, and shows the
// count at once: the first frame of a tray has no event to wait for.
func (t *trayCount) attach(icon trayIcon) {
	t.icon = icon
	n := t.count()
	icon.SetStatus(trayStatus(n))
	icon.SetDot(n > 0)
}

// closeToTray is the main window's OnClose listener: frostui's tray
// decides with this computer's setting, and a close it does not take
// goes on as it did without a tray.
func closeToTray(t interface {
	CloseToTray(*mygo.CloseEvent, bool) bool
}, enabled func() bool) func(*mygo.CloseEvent) {
	return func(e *mygo.CloseEvent) { t.CloseToTray(e, enabled()) }
}

// trayHandle is frostui's Tray as its wiring needs it.
type trayHandle interface {
	trayIcon
	CloseToTray(*mygo.CloseEvent, bool) bool
	Destroy()
}

// trayWindow is the main window as the wiring needs it; *mygo.Window
// satisfies it, and a test gives a fake.
type trayWindow interface {
	OnClose(func(*mygo.CloseEvent)) func()
	OnClosed(func()) func()
}

// traySetting is this computer's close-to-tray setting, as the settings
// window holds it.
type traySetting interface {
	CloseToTray() bool
	SetTray(bool)
}

// wireTray puts the tray on the main window: it shows the count now, sets
// the close's rule, and takes the icon away with the window. The app runs
// on after the main window closes while a compose window is open, so a
// tray left behind would offer Open for a window that is gone.
//
// A count posted after Destroy is safe: frostui's Tray keeps its status
// and dot in fields and skips an icon it no longer has.
func wireTray(win trayWindow, tray trayHandle, tc *trayCount, setting traySetting) {
	tc.attach(tray)
	setting.SetTray(true)
	win.OnClose(closeToTray(tray, setting.CloseToTray))
	win.OnClosed(func() {
		tray.Destroy()
		setting.SetTray(false)
	})
}

// raiser is a window that can come to the front; *mygo.Window satisfies
// it, and a test gives a fake.
type raiser interface {
	Show()
	Restore()
	Focus()
}

// raise shows and focuses the main window, after close to tray hid it or
// a second instance asked for it.
func raise(w raiser) {
	w.Show()
	w.Restore()
	w.Focus()
}
