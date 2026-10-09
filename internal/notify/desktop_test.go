package notify

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/dbustest"
	"github.com/godbus/dbus/v5"
)

// fakeServer is org.freedesktop.Notifications on a private bus.
type fakeServer struct {
	conn   *dbus.Conn
	mu     sync.Mutex
	next   uint32
	calls  []fakeCall
	closed []uint32
}

func (f *fakeServer) CloseNotification(id uint32) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	return nil
}

type fakeCall struct {
	replaces         uint32
	summary, body    string
	actions          []string
	entry, icon, app string
	category         string
}

func (f *fakeServer) Notify(app string, replaces uint32, icon, summary, body string, actions []string,
	hints map[string]dbus.Variant, _ int32) (uint32, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, _ := hints["desktop-entry"].Value().(string)
	category, _ := hints["category"].Value().(string)
	f.calls = append(f.calls, fakeCall{replaces: replaces, summary: summary, body: body, actions: actions, entry: entry, icon: icon,
		app: app, category: category})
	if replaces != 0 {
		return replaces, nil
	}
	f.next++
	return f.next, nil
}

func (f *fakeServer) emit(t *testing.T, member string, body ...any) {
	t.Helper()
	if err := f.conn.Emit(busPath, busIface+"."+member, body...); err != nil {
		t.Fatal(err)
	}
}

func startFake(t *testing.T) (*fakeServer, string) {
	t.Helper()
	addr := dbustest.Bus(t)
	f := &fakeServer{conn: dbustest.Connect(t, addr)}
	if err := f.conn.ExportMethodTable(map[string]any{"Notify": f.Notify, "CloseNotification": f.CloseNotification}, busPath, busIface); err != nil {
		t.Fatal(err)
	}
	if reply, err := f.conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own %s: %v %v", busName, reply, err)
	}
	return f, addr
}

func TestDesktopAnnouncesAndOpens(t *testing.T) {
	f, addr := startFake(t)
	opened := make(chan int64, 4)
	d, err := NewDesktop(t.Context(), dbustest.Connect(t, addr), slog.New(slog.DiscardHandler), func(id int64) { opened <- id })
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := d.Announce(ctx, 1, []Mail{{ID: 41, FromName: "Ann", Subject: "Lunch", Preview: "Noon?"}}); err != nil {
		t.Fatal(err)
	}
	many := []Mail{{ID: 1, FromName: "A"}, {ID: 2, FromName: "B"}, {ID: 3, FromName: "C"}, {ID: 4, FromName: "D"}}
	for range 2 {
		if err := d.Announce(ctx, 1, many); err != nil {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	calls := append([]fakeCall(nil), f.calls...)
	f.mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("calls = %+v", calls)
	}
	first := calls[0]
	if first.summary != "Ann" || first.app != "Frostmail" || first.icon != "org.frostyard.Frostmail" || first.entry != "org.frostyard.Frostmail" ||
		len(first.actions) != 2 || first.actions[0] != "default" || first.replaces != 0 {
		t.Errorf("message note = %+v", first)
	}
	if calls[1].summary != "4 new messages" || calls[1].replaces != 0 || calls[2].replaces != 2 {
		t.Errorf("group notes = %+v, %+v; the second must replace the first", calls[1], calls[2])
	}

	// A click from another program on the bus is ignored: it clicks a note
	// of message 42, and its round trip to the bus makes sure the bus has
	// routed that signal before the server's own click below.
	if err := d.Announce(ctx, 2, []Mail{{ID: 42, FromName: "Bob"}}); err != nil {
		t.Fatal(err)
	}
	impostor := dbustest.Connect(t, addr)
	if err := impostor.Emit(busPath, busIface+".ActionInvoked", uint32(3), "default"); err != nil {
		t.Fatal(err)
	}
	if err := impostor.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetId", 0).Err; err != nil {
		t.Fatal(err)
	}

	// Clicking the message note opens its message; a closed note is forgotten.
	f.emit(t, "ActionInvoked", uint32(1), "default")
	select {
	case id := <-opened:
		if id != 41 {
			t.Errorf("opened message %d, want 41 (42 is the impostor's)", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clicking the notification opened nothing")
	}
	f.emit(t, "NotificationClosed", uint32(2), uint32(2))
	f.emit(t, "ActionInvoked", uint32(2), "default")
	f.emit(t, "ActionInvoked", uint32(99), "default")
	f.emit(t, "ActionInvoked", uint32(1), "other")
	// A known click after them shows they were handled and ignored.
	f.emit(t, "ActionInvoked", uint32(1), "default")
	select {
	case id := <-opened:
		if id != 41 {
			t.Errorf("opened %d after closed or unknown notes; want only 41", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second click opened nothing")
	}

}
