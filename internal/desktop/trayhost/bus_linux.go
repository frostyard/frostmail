//go:build linux

package trayhost

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
)

// nameHasOwner calls org.freedesktop.DBus.NameHasOwner on the session bus,
// on a connection of its own that it closes. It never autolaunches a bus:
// with no DBUS_SESSION_BUS_ADDRESS it takes $XDG_RUNTIME_DIR/bus, and with
// neither it fails.
func nameHasOwner(name string) (bool, error) {
	addr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if addr == "" {
		dir := os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			return false, errors.New("trayhost: no session bus address")
		}
		addr = "unix:path=" + dbus.EscapeBusAddressValue(filepath.Join(dir, "bus"))
	}
	conn, err := dbus.Connect(addr) // Auth and Hello
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&owned)
	return owned, err
}
