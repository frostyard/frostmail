package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// The D-Bus sender (docs/design/desktop.md, Notifications): new mail goes
// to org.freedesktop.Notifications, and clicking a notification opens its
// message in the app.

const (
	busName   = "org.freedesktop.Notifications"
	busPath   = dbus.ObjectPath("/org/freedesktop/Notifications")
	busIface  = "org.freedesktop.Notifications"
	appName   = "Frostmail"
	appID     = "org.frostyard.Frostmail" // the desktop entry and icon (ADR-0013)
	callLimit = 5 * time.Second
)

// Desktop sends notifications over a session bus and runs open with the
// message of a clicked one (0 for a group note).
type Desktop struct {
	conn *dbus.Conn
	log  *slog.Logger
	open func(messageID int64)

	mu     sync.Mutex
	groups map[int64]uint32 // account → its last group note, which the next replaces
	opens  map[uint32]int64 // notification → the message it opens

	reminders     map[uint32]string // notification → its reminder
	reminderNotes map[string]uint32 // reminder → its notification
	onReminder    func(id, action string)
}

// NewDesktop sends over conn and watches it for clicks until ctx ends.
func NewDesktop(ctx context.Context, conn *dbus.Conn, log *slog.Logger, open func(messageID int64)) (*Desktop, error) {
	d := &Desktop{conn: conn, log: log, open: open, groups: map[int64]uint32{}, opens: map[uint32]int64{},
		reminders: map[uint32]string{}, reminderNotes: map[string]uint32{}}
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		if err := conn.AddMatchSignalContext(ctx, dbus.WithMatchSender(busName), dbus.WithMatchObjectPath(busPath),
			dbus.WithMatchInterface(busIface), dbus.WithMatchMember(member)); err != nil {
			return nil, fmt.Errorf("watch notifications: %w", err)
		}
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	go d.watch(ctx, signals)
	return d, nil
}

// OpenDesktop connects to the session bus.
func OpenDesktop(ctx context.Context, log *slog.Logger, open func(messageID int64)) (*Desktop, error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	d, err := NewDesktop(ctx, conn, log, open)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return d, nil
}

// Close disconnects from the bus.
func (d *Desktop) Close() error { return d.conn.Close() }

// Announce shows the notes of one account's new mail (Notes). A group note
// replaces the account's previous group note.
func (d *Desktop) Announce(ctx context.Context, accountID int64, mail []Mail) error {
	for _, n := range Notes(mail) {
		var replaces uint32
		if n.MessageID == 0 {
			d.mu.Lock()
			replaces = d.groups[accountID]
			d.mu.Unlock()
		}
		id, err := d.send(ctx, replaces, n)
		if err != nil {
			return err
		}
		d.mu.Lock()
		if n.MessageID == 0 {
			d.groups[accountID] = id
		}
		d.opens[id] = n.MessageID
		d.mu.Unlock()
	}
	return nil
}

func (d *Desktop) send(ctx context.Context, replaces uint32, n Note) (uint32, error) {
	ctx, cancel := context.WithTimeout(ctx, callLimit)
	defer cancel()
	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant(appID),
		"category":      dbus.MakeVariant("email.arrived"),
	}
	var id uint32
	err := d.conn.Object(busName, busPath).CallWithContext(ctx, busIface+".Notify", 0,
		appName, replaces, appID, n.Summary, n.Body, []string{"default", "Open"}, hints, int32(-1)).Store(&id)
	if err != nil {
		return 0, fmt.Errorf("notify: %w", err)
	}
	return id, nil
}

// watch runs open for clicked notifications and forgets closed ones.
func (d *Desktop) watch(ctx context.Context, signals <-chan *dbus.Signal) {
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-signals:
			if !ok {
				return
			}
			d.handle(s)
		}
	}
}

func (d *Desktop) handle(s *dbus.Signal) {
	if s.Path != busPath || len(s.Body) < 2 {
		return
	}
	id, ok := s.Body[0].(uint32)
	if !ok {
		return
	}
	action, _ := s.Body[1].(string)
	if d.reminderAction(id, action, s.Name == busIface+".NotificationClosed") {
		return
	}
	d.mu.Lock()
	msg, known := d.opens[id]
	if s.Name == busIface+".NotificationClosed" {
		delete(d.opens, id)
	}
	d.mu.Unlock()
	if s.Name == busIface+".ActionInvoked" && action == "default" && known {
		d.log.Debug("notification clicked", "message", msg)
		d.open(msg)
	}
}
