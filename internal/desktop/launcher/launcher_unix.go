//go:build !windows

// Package launcher shows the All Inboxes count on the app's launcher icon.
//
// It sends the Unity LauncherEntry signal,
// com.canonical.Unity.LauncherEntry.Update, on the session bus. KDE
// Plasma, Ubuntu's dock, Dash to Dock and Plank read it, so the badge
// follows the sidebar's All Inboxes row wherever the icon is.
//
// The app does not use MyGo's App.SetBadgeCount. MyGo names the desktop
// entry after the executable (frostmail-app.desktop in the Flatpak) unless
// the launcher set GIO_LAUNCHED_DESKTOP_FILE, so a dock would not match
// org.frostyard.Frostmail.desktop when the app starts from a terminal,
// autostart or a notification; and MyGo's fake backend cannot show the
// signal in a test.
//
// A broadcast signal, one with no destination, needs no Flatpak
// permission: xdg-dbus-proxy gives every client TALK to a NULL destination
// (flatpak/xdg-dbus-proxy, flatpak-proxy.c,
// flatpak_proxy_client_get_max_policy_and_matched,
// https://github.com/flatpak/xdg-dbus-proxy/blob/72b40c8f6d9d585cfbdb249690724f740d207087/flatpak-proxy.c#L1919-L1925).
package launcher

import (
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/frostyard/frostmail/internal/config"
)

const (
	// iface and member name the Unity LauncherEntry API.
	iface  = "com.canonical.Unity.LauncherEntry"
	member = "Update"
	// signalName is the signal's D-Bus name.
	signalName = iface + "." + member
	// appURI is the entry's app URI, the desktop entry of ADR-0013.
	appURI = "application://" + config.AppID + ".desktop"
	// closeLimit is how long Close waits for the last signal.
	closeLimit = time.Second
)

// entryPath is libunity's object path for the entry: the decimal
// g_str_hash of the URI.
var entryPath = dbus.ObjectPath("/com/canonical/unity/launcherentry/" + strconv.FormatUint(uint64(strHash(appURI)), 10))

// strHash is glib's g_str_hash, djb2 on the bytes: h = h*33 + c.
func strHash(s string) uint32 {
	h := uint32(5381)
	for i := 0; i < len(s); i++ {
		h = h*33 + uint32(s[i])
	}
	return h
}

// Entry is the app's launcher entry: the count it shows, kept up to date by
// one goroutine.
type Entry struct {
	log   *slog.Logger
	count func() int64
	emit  func(path dbus.ObjectPath, name string, values ...any) error
	bus   func() error

	changed chan struct{} // a count may have changed; buffer 1 coalesces
	quit    chan struct{} // closed by Close: the worker clears and exits
	done    chan struct{} // closed when the worker has exited
	failed  bool          // an emit failed and none has succeeded since
	once    sync.Once
}

// Open connects to the session bus and returns the entry; count reads the
// count to show, on the entry's own goroutine. With no session bus the
// entry does nothing: no error, one Debug log line.
func Open(log *slog.Logger, count func() int64) *Entry {
	if log == nil {
		log = slog.Default()
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Debug("launcher count", "err", err)
		return newEntry(log, count, nil, nil)
	}
	return newEntry(log, count, conn.Emit, conn.Close)
}

// newEntry starts the entry's goroutine over emit and closeBus; nil emit
// makes an inert entry.
func newEntry(log *slog.Logger, count func() int64, emit func(path dbus.ObjectPath, name string, values ...any) error,
	closeBus func() error) *Entry {
	e := &Entry{log: log, count: count, emit: emit, bus: closeBus}
	if emit == nil {
		return e
	}
	e.changed = make(chan struct{}, 1)
	e.quit = make(chan struct{})
	e.done = make(chan struct{})
	go e.work()
	return e
}

// Changed says the count may have changed. It never blocks; it may run on
// any goroutine.
func (e *Entry) Changed() {
	if e.changed == nil {
		return
	}
	select {
	case e.changed <- struct{}{}:
	default: // a wake is pending, and it reads the latest count
	}
}

// Close clears the badge (count 0, count-visible false), stops the
// goroutine and closes the bus. It waits at most closeLimit (1 s) for the
// last signal. It is safe to call more than once and on an inert entry.
func (e *Entry) Close() {
	e.once.Do(func() {
		if e.emit == nil {
			return
		}
		close(e.quit)
		select {
		case <-e.done:
		case <-time.After(closeLimit):
		}
		if e.bus != nil {
			_ = e.bus()
		}
	})
}

// work is the entry's one goroutine: it sends the count when it changes,
// and the clear signal on Close.
func (e *Entry) work() {
	defer close(e.done)
	var last int64
	sent := false
	for {
		select {
		case <-e.changed:
			n := e.count()
			if sent && n == last {
				continue // the dock already shows this count
			}
			e.send(n)
			last, sent = n, true
		case <-e.quit:
			e.send(0)
			return
		}
	}
}

// send emits one Update: the count, and whether the dock shows it.
func (e *Entry) send(n int64) {
	if n < 0 {
		n = 0
	}
	err := e.emit(entryPath, signalName, appURI, map[string]dbus.Variant{
		"count":         dbus.MakeVariant(n),
		"count-visible": dbus.MakeVariant(n > 0),
	})
	if err != nil {
		if !e.failed { // one line, then quiet until an emit succeeds again
			e.failed = true
			e.log.Debug("launcher count", "err", err)
		}
		return
	}
	e.failed = false
}
