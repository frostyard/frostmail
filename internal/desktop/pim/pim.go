// Package pim is the desktop app's Calendar, People and Tasks modules,
// the To-Do Bar, the event window and the reminders window
// (docs/specs/pim-ui.md; plan 0015, Phase 7). All of its state belongs to
// the UI thread; goroutines hand results back through Options.Post.
package pim

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// The modules' pane IDs. The sidebar's is the main window's, so its
// toggle and width are shared.
const (
	paneSidebar  = "sidebar"
	paneCalView  = "calendar-view"
	paneCalEvent = "calendar-event"
	panePeople   = "people-list"
	panePerson   = "person"
	paneTasks    = "tasks-list"
	paneTask     = "task"

	sidebarWidth = 220
	listWidth    = 360
	detailWidth  = 320

	// actionTimeout bounds one write to maild; refreshDelay is how long
	// events are gathered before a refetch; undoLimit how many writes a
	// module can undo.
	actionTimeout = 60 * time.Second
	refreshDelay  = 100 * time.Millisecond
	undoLimit     = 20
)

// WindowSpec describes a window the module asks the app to show.
type WindowSpec struct {
	// Key identifies the window: OpenWindow shows it or raises it.
	Key, Title                         string
	Width, Height, MinWidth, MinHeight float32
	AlwaysOnTop                        bool
	// Frameless asks for no system title bar: the view draws its own strip.
	Frameless bool
	// View draws the window.
	View func(c *ui.Context)
	// OnClose runs when the window has closed, however it closed.
	OnClose func()
	// CloseRequested, when set, runs as the user asks to close the window
	// with the system's button; false keeps the window open, as for
	// "Save changes?".
	CloseRequested func() bool
}

// Options configure a Module.
type Options struct {
	// Post runs a function on the UI thread and redraws; Wake redraws.
	Post func(func())
	Wake func()
	// Now is the clock and, by its Location, the app's zone [time.Now].
	Now func() time.Time
	// Dialogs shows the main window's questions.
	Dialogs *frostui.Dialogs
	// Toast shows a notice, with an error for a failure.
	Toast func(title string, err error)
	// Compose starts a new message; OpenMessage shows one in Mail.
	Compose     func(address, name string)
	OpenMessage func(id int64)
	// OpenWindow shows or raises a window by Key.
	OpenWindow func(WindowSpec)
	// CloseWindow closes a window by Key; without it, a window cannot
	// close itself.
	CloseWindow func(key string)
	// SetWindowTitle retitles a window by Key; nil leaves it.
	SetWindowTitle func(key, title string)
	// ShowModule switches the main window's module; nil leaves it.
	ShowModule func(commands.Module)
	// OpenImage asks for an image, on a worker; "" for cancelled.
	OpenImage func() (string, error)
	// ContactCard shows an address's contact card; without it, a name
	// shows the address's person in People.
	ContactCard func(address, name string)
}

// Module is the three modules' state.
type Module struct {
	o Options

	mu     sync.Mutex
	timers map[string]*time.Timer
	off    func()
	cancel context.CancelFunc

	// The UI thread's.
	client   *api.Client
	ctx      context.Context
	events   *live.Events
	conn     int
	accounts fetch[[]api.Account]
	clock    clock
	zone     string
	cal      calendarState
	people   peopleState
	tasks    tasksState
	todo     todoState
	remind   remindersState
	windows  map[string]*eventWindow
	newCount int
}

// New returns a module that is not connected yet; Connect connects it.
func New(o Options) *Module {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Toast == nil {
		o.Toast = func(string, error) {}
	}
	if o.Wake == nil {
		o.Wake = func() {}
	}
	if o.Dialogs == nil {
		o.Dialogs = &frostui.Dialogs{}
	}
	m := &Module{o: o, timers: map[string]*time.Timer{}, windows: map[string]*eventWindow{}, ctx: context.Background()}
	m.clock = clock{loc: o.Now().Location()}
	m.zone = zoneName(m.clock.loc)
	m.setUpCalendar()
	m.setUpPeople()
	m.setUpTasks()
	return m
}

// Connect keeps the module current from c and events until the next
// Connect or Disconnect. It raises the reminders if any are due.
func (m *Module) Connect(ctx context.Context, c *api.Client, events *live.Events) error {
	m.stop()
	own, cancel := context.WithCancel(context.Background())
	off := events.On(m.event)
	m.mu.Lock()
	m.off, m.cancel = off, cancel
	m.mu.Unlock()
	due, err := c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
	if err != nil {
		return fmt.Errorf("list reminders: %w", err)
	}
	m.o.Post(func() {
		m.client, m.ctx, m.events = c, own, events
		m.conn++
		m.closeFlagged()
		if len(due) > 0 {
			m.raiseReminders()
		}
	})
	return nil
}

// Disconnect stops following the connection; any goroutine may call it.
func (m *Module) Disconnect() {
	m.stop()
	m.o.Post(func() {
		m.client, m.events = nil, nil
		m.closeFlagged()
	})
}

func (m *Module) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.off != nil {
		m.off()
		m.off = nil
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	for k, t := range m.timers {
		t.Stop()
		delete(m.timers, k)
	}
}

// debounce runs fn on the UI thread once events of key stop coming.
func (m *Module) debounce(key string, fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.timers[key]; t != nil {
		t.Stop()
	}
	m.timers[key] = time.AfterFunc(refreshDelay, func() { m.o.Post(fn) })
}

// event keeps the module current; it runs on the events' goroutine.
func (m *Module) event(ev api.Event) {
	switch e := ev.(type) {
	case api.CalendarChanged:
		m.debounce("calendar", m.calendarChanged)
	case api.CalendarReminders:
		m.o.Post(func() { m.remindersEvent(e.Count) })
	case api.CalendarRefused:
		m.o.Post(func() { m.o.Toast(refusedText(e.Summary, e.Reason), nil) })
	case api.PeopleChanged:
		m.debounce("people", m.peopleChanged)
	case api.PeopleRefused:
		m.o.Post(func() { m.o.Toast(refusedText(e.Name, e.Reason), nil) })
	case api.TasksChanged:
		m.debounce("tasks", m.tasksChanged)
	case api.AccountChanged:
		m.debounce("account", m.accountChanged)
	}
}

func (m *Module) accountChanged() {
	m.accounts.dirty = true
	m.cal.collections.dirty = true
	m.people.books.dirty = true
	m.tasks.lists.dirty = true
}

// refusedText words a change the server refused (docs/specs/pim-ui.md,
// People: Refused changes).
func refusedText(name, reason string) string {
	switch reason {
	case "changed":
		return "“" + name + "” was changed on another device, so your change wasn't saved."
	case "deleted":
		return "“" + name + "” was deleted on another device."
	}
	return "The server didn't accept your change to “" + name + "”."
}

// fetch keeps one call's answer: a new key or Invalidate asks again, and
// an answer overtaken by another is dropped.
type fetch[T any] struct {
	key     string
	started bool
	dirty   bool
	gen     int
	pending bool
	value   T
	err     error
	loaded  bool
	version int
}

// want asks for key's answer, when it is not the one held.
func want[T any](m *Module, f *fetch[T], key string, load func(ctx context.Context, c *api.Client) (T, error)) {
	if m.client == nil {
		return
	}
	key = fmt.Sprint(m.conn, "|", key)
	if f.started && f.key == key && !f.dirty {
		return
	}
	f.key, f.started, f.dirty, f.err, f.pending = key, true, false, nil, true
	f.gen++
	gen, c, ctx := f.gen, m.client, m.ctx
	go func() {
		v, err := load(ctx, c)
		m.o.Post(func() {
			if f.gen != gen {
				return
			}
			f.pending = false
			if ctx.Err() != nil { // the connection went: the next one asks again
				f.dirty = true
				return
			}
			if err == nil {
				f.value = v
			}
			f.err, f.loaded = err, true
			f.version++
		})
	}()
}

// run does a write off the UI thread; a failure shows as a toast and then
// failed, if set, runs on the UI thread.
func (m *Module) run(what string, fn func(ctx context.Context, c *api.Client) error, failed func()) {
	c := m.client
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		if err := fn(ctx, c); err != nil {
			slog.Warn(what, "err", err)
			m.o.Post(func() {
				m.o.Toast("Could not "+what, err)
				if failed != nil {
					failed()
				}
			})
		}
	}()
}

// frame reads what changes between frames, once per draw.
func (m *Module) frame(c *ui.Context) {
	m.clock.loc = m.o.Now().Location()
	m.zone = zoneName(m.clock.loc)
	r := c.Region()
	m.clock.h24 = r.HourCycle == 24
	if r.HourCycle != 0 {
		m.clock.weekStart = int(r.FirstWeekday)
	}
	now := m.o.Now()
	c.After(now.Truncate(time.Minute).Add(time.Minute).Sub(now))
	want(m, &m.accounts, "accounts", func(ctx context.Context, c *api.Client) ([]api.Account, error) {
		return c.Account().List(ctx, &api.AccountListParams{})
	})
	if m.cal.date == "" {
		m.cal.date = m.today()
	}
}

func (m *Module) now() time.Time { return m.o.Now() }

func (m *Module) today() string { return todayIn(m.now(), m.clock.loc) }

func (m *Module) account(id int64) *api.Account {
	for i := range m.accounts.value {
		if m.accounts.value[i].ID == id {
			return &m.accounts.value[i]
		}
	}
	return nil
}

func (m *Module) accountEmail(id int64) string {
	if a := m.account(id); a != nil {
		return a.Email
	}
	return ""
}

func (m *Module) accountReadOnly(id int64) bool {
	a := m.account(id)
	return a != nil && a.ReadOnly
}

func (m *Module) show(module commands.Module) {
	if m.o.ShowModule != nil {
		m.o.ShowModule(module)
	}
}

// Panes draws a module's panes: Calendar, People or Tasks.
func (m *Module) Panes(c *ui.Context, module commands.Module) []frostui.Pane {
	m.frame(c)
	switch module {
	case commands.Calendar:
		return m.calendarPanes(c)
	case commands.People:
		return m.peoplePanes(c)
	case commands.Tasks:
		return m.tasksPanes(c)
	}
	return nil
}

// Handlers are a module's commands.
func (m *Module) Handlers(module commands.Module) map[commands.Command]func() {
	switch module {
	case commands.Calendar:
		return m.calendarHandlers()
	case commands.People:
		return m.peopleHandlers()
	case commands.Tasks:
		return m.tasksHandlers()
	}
	return nil
}

// Selected is how many items a module has selected.
func (m *Module) Selected(module commands.Module) int {
	selected := false
	switch module {
	case commands.Calendar:
		selected = m.cal.selected != nil
	case commands.People:
		selected = m.people.selected != 0
	case commands.Tasks:
		selected = m.tasks.selected != 0
	}
	if selected {
		return 1
	}
	return 0
}

// Inert reports whether the main window must keep its module keys and bar
// still: a contact is being edited (docs/specs/pim-ui.md, Editing a
// contact).
func (m *Module) Inert() bool { return m.people.editing != nil }

// localZone stands for the system's zone when it cannot be named:
// time.LoadLocation reads it back as time.Local.
const localZone = "Local"

// zoneName is the IANA name of a zone, or localZone when the system does
// not say one.
func zoneName(loc *time.Location) string {
	if name := loc.String(); name != localZone {
		return name
	}
	return calendar.LocalZone().String()
}

// wireZone is the zone a write names: an event's instants stay right in
// UTC when the system's zone has no IANA name.
func wireZone(zone string) string {
	if zone == localZone || zone == "" {
		return "UTC"
	}
	return zone
}

// zonePtr names the app's zone for a call, nil for maild's local zone.
func (m *Module) zonePtr() *string {
	if m.zone == localZone {
		return nil
	}
	z := m.zone
	return &z
}

func decodePhoto(p *api.Photo) *ui.Bitmap {
	if p == nil {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil
	}
	b, err := uikit.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	return b
}
