package pim

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// searchPause is how long the search field waits after the last key.
const searchPause = 250 * time.Millisecond

// peopleRow is a row of the People list: a letter, or a person.
type peopleRow struct {
	letter string
	person *api.PersonSummary
}

// personPhoto is a person's decoded photo.
type personPhoto struct {
	id     int64
	bitmap *ui.Bitmap
}

// peopleState is the People module's state.
type peopleState struct {
	books    fetch[[]api.Collection]
	list     fetch[[]api.PersonSummary]
	person   fetch[*api.Person]
	card     fetch[*api.ContactCard]
	photo    fetch[personPhoto]
	source   string // "all" or "book:<id>"
	query    string
	draft    string
	token    int
	focusSrc bool
	selected int64
	before   int64 // the person selected before New Contact
	focus    string
	focusAsk string
	sources  frostui.SourceList[string]
	listUI   ui.ListState
	row      int
	rows     []peopleRow
	rowsAt   int
	editing  *contactEditor
	undo     []string
	// restoring is set while a People or Calendar undo runs (undoNewest).
	restoring bool
}

func (m *Module) setUpPeople() {
	st := &m.people
	st.source, st.row, st.focus = "all", -1, panePeople
	st.sources.Label = "Address Books"
	st.sources.Row = func(key string) frostui.SourceRow { return m.bookRow(key) }
	st.sources.OnSelect = m.setBook
}

func (m *Module) peopleChanged() {
	m.people.list.dirty, m.people.person.dirty, m.people.card.dirty, m.people.photo.dirty = true, true, true, true
}

func (m *Module) setBook(key string) {
	st := &m.people
	if st.source != key {
		st.source = key
	}
}

func (m *Module) book(id int64) *api.Collection {
	for i := range m.people.books.value {
		if m.people.books.value[i].ID == id {
			return &m.people.books.value[i]
		}
	}
	return nil
}

func bookKey(id int64) string { return "book:" + strconv.FormatInt(id, 10) }

func (m *Module) bookRow(key string) frostui.SourceRow {
	if key == "all" {
		return frostui.SourceRow{Title: "All Contacts", Icon: "system-users"}
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(key, "book:"), 10, 64)
	b := m.book(id)
	if b == nil {
		return frostui.SourceRow{}
	}
	row := frostui.SourceRow{Title: b.Name, Icon: "x-office-address-book"}
	if b.ReadOnly || m.accountReadOnly(b.AccountID) {
		row.Tag, row.Spoken = "🔒", "Read-only"
	}
	return row
}

// syncPeople asks for what the module shows.
func (m *Module) syncPeople() {
	st := &m.people
	want(m, &st.books, "books", func(ctx context.Context, c *api.Client) ([]api.Collection, error) {
		kind := api.CollectionKindAddressbook
		return c.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
	})
	source, query := st.source, st.query
	want(m, &st.list, source+"|"+query, func(ctx context.Context, c *api.Client) ([]api.PersonSummary, error) {
		p := &api.PeopleListParams{}
		if query != "" {
			p.Query = &query
		}
		if id, ok := strings.CutPrefix(source, "book:"); ok {
			n, _ := strconv.ParseInt(id, 10, 64)
			p.CollectionID = &n
		}
		return c.People().List(ctx, p)
	})
	if st.rowsAt != st.list.version {
		st.rowsAt = st.list.version
		st.rows = peopleRows(st.list.value)
	}
	m.syncPerson()
}

// peopleRows puts a letter before the first person of each index.
func peopleRows(people []api.PersonSummary) []peopleRow {
	var rows []peopleRow
	previous, first := "", true
	for i := range people {
		p := &people[i]
		if first || p.Index != previous {
			rows = append(rows, peopleRow{letter: p.Index})
		}
		rows = append(rows, peopleRow{person: p})
		previous, first = p.Index, false
	}
	return rows
}

// syncPerson follows the selection across refetches.
func (m *Module) syncPerson() {
	st := &m.people
	st.row = -1
	found := false
	for i, r := range st.rows {
		if r.person != nil && r.person.ID == st.selected {
			st.row, found = i, true
			break
		}
	}
	if st.selected != 0 && !found && st.list.loaded && !st.list.pending && !st.list.dirty && st.editing == nil {
		st.selected = 0
	}
	if st.selected == 0 {
		return
	}
	id := st.selected
	want(m, &st.person, strconv.FormatInt(id, 10), func(ctx context.Context, c *api.Client) (*api.Person, error) {
		return c.People().Get(ctx, &api.PeopleGetParams{ID: id})
	})
	p := m.shownPerson()
	if p == nil {
		return
	}
	email := firstEmail(p)
	if email != "" {
		want(m, &st.card, email, func(ctx context.Context, c *api.Client) (*api.ContactCard, error) {
			return c.People().Card(ctx, &api.PeopleCardParams{Email: email})
		})
	}
	if p.HasPhoto {
		want(m, &st.photo, strconv.FormatInt(id, 10), func(ctx context.Context, c *api.Client) (personPhoto, error) {
			photo, err := c.People().Photo(ctx, &api.PeoplePhotoParams{ID: id})
			if err != nil {
				return personPhoto{}, err
			}
			return personPhoto{id, decodePhoto(photo)}, nil
		})
	}
}

func (m *Module) shownPerson() *api.Person {
	p := m.people.person.value
	if p == nil || p.ID != m.people.selected {
		return nil
	}
	return p
}

// showsContact reports whether the shown person holds a contact. A relink
// can move the contact being edited to another person, and the editor,
// which keeps the list inert, must stay on screen.
func (m *Module) showsContact(id int64) bool {
	p := m.shownPerson()
	return p != nil && slices.ContainsFunc(p.Contacts, func(c api.Contact) bool { return c.ID == id })
}

func firstEmail(p *api.Person) string {
	for _, c := range p.Contacts {
		if len(c.Emails) > 0 {
			return c.Emails[0].Value
		}
	}
	return ""
}

// ShowPerson selects a person from elsewhere, showing every contact.
func (m *Module) ShowPerson(id int64) {
	st := &m.people
	st.source, st.query, st.draft = "all", "", ""
	st.selected = id
	st.sources.Select("", "all")
	m.show(commands.People)
}

func (m *Module) peopleHandlers() map[commands.Command]func() {
	st := &m.people
	focusSearch := func() {
		if st.editing == nil {
			st.focusSrc = true
		}
	}
	return map[commands.Command]func(){
		commands.Compose:      m.newContact,
		commands.FocusSearch:  focusSearch,
		commands.Find:         focusSearch,
		commands.Undo:         m.undoPeople,
		commands.Escape:       m.peopleEscape,
		commands.NextPane:     func() { m.cyclePeople(1) },
		commands.PreviousPane: func() { m.cyclePeople(-1) },
	}
}

func (m *Module) peopleEscape() {
	st := &m.people
	switch {
	case st.editing != nil:
		st.editing.cancel()
	case st.draft != "" || st.query != "":
		st.token++
		st.draft, st.query = "", ""
	}
}

func (m *Module) cyclePeople(step int) {
	st := &m.people
	if st.editing != nil {
		return
	}
	panes := []string{paneSidebar, panePeople, panePerson}
	i := max(slices.Index(panes, st.focus), 0)
	st.focusAsk = panes[(i+step+len(panes))%len(panes)]
	st.focus = st.focusAsk
}

func (m *Module) undoPeople() {
	m.undoNewest(&m.people.undo, func(ctx context.Context, c *api.Client, restore string, _ *bool) error {
		return c.People().Restore(ctx, &api.PeopleRestoreParams{RestoreID: restore})
	})
}

func (m *Module) peoplePanes(c *ui.Context) []frostui.Pane {
	m.syncPeople()
	return []frostui.Pane{
		{ID: paneSidebar, Label: "Address Books", Width: sidebarWidth, Min: 160, Max: 320, Collapsible: true, Content: func() { m.peopleSidebar(c) }},
		{ID: panePeople, Label: "Contacts", Width: listWidth, Min: 280, Max: 560, Bar: func() { m.peopleBar(c) }, Content: func() { m.peopleList(c) }},
		{ID: panePerson, Label: "Contact", Min: 360, Bar: func() { m.personBar(c) }, Content: func() { m.personPane(c) }},
	}
}

// writableBooks are the address books that can take a contact.
func (m *Module) writableBooks() []api.Collection {
	var out []api.Collection
	for _, b := range m.people.books.value {
		if !b.ReadOnly && !m.accountReadOnly(b.AccountID) {
			out = append(out, b)
		}
	}
	return out
}

func (m *Module) peopleSidebar(c *ui.Context) {
	st := &m.people
	sections := []frostui.SourceSection[string]{{ID: "people", Title: "People", Items: []string{"all"}}}
	for _, a := range m.accounts.value {
		var items []string
		for _, b := range st.books.value {
			if b.AccountID == a.ID {
				items = append(items, bookKey(b.ID))
			}
		}
		if len(items) > 0 {
			sections = append(sections, frostui.SourceSection[string]{ID: fmt.Sprint("account:", a.ID), Title: a.Email, Items: items})
		}
	}
	if key, _, ok := st.sources.Selected(); !ok || key != st.source {
		st.sources.Select("", st.source)
	}
	inert := st.editing != nil
	ui.Column(c).Grow(1).AlignItems(ui.Stretch).Background(frostui.Chrome(c.Theme())).IgnoreInput(inert).Children(func() {
		tree := st.sources.View(c, sections).Grow(1)
		if st.focusAsk == paneSidebar {
			st.focusAsk = ""
			tree.Focus()
		}
		if tree.FocusWithin() {
			st.focus = paneSidebar
		}
	})
}

func (m *Module) peopleTitle() string {
	st := &m.people
	if id, ok := strings.CutPrefix(st.source, "book:"); ok {
		n, _ := strconv.ParseInt(id, 10, 64)
		if b := m.book(n); b != nil {
			return b.Name
		}
	}
	return "All Contacts"
}

func (m *Module) peopleSubtitle() string {
	n := len(m.people.list.value)
	switch {
	case m.people.query != "":
		return fmt.Sprintf("%d results", n)
	case n == 1:
		return "1 contact"
	}
	return fmt.Sprintf("%d contacts", n)
}

// peopleBar is the top bar's segment over the list.
func (m *Module) peopleBar(c *ui.Context) {
	st := &m.people
	ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
		frostui.Strong(c, m.peopleTitle()).SingleLine()
		frostui.Caption(c, m.peopleSubtitle()).SingleLine()
	})
	if frostui.IconButton(c, "contact-new", "New Contact (Ctrl+N)").Disabled(len(m.writableBooks()) == 0 || st.editing != nil).Clicked() {
		m.newContact()
	}
	if frostui.IconButton(c, "mail-message-new", "New Message").Clicked() && m.o.Compose != nil {
		address, name := "", ""
		if p := m.shownPerson(); p != nil {
			address, name = firstEmail(p), p.DisplayName
		}
		m.o.Compose(address, name)
	}
}

// personBar is the segment over the person pane: the search field.
func (m *Module) personBar(c *ui.Context) {
	st := &m.people
	ui.Spacer(c)
	f := ui.SearchField(c, &st.draft).Width(220).Label("Search Contacts").Placeholder("Search Contacts").Disabled(st.editing != nil)
	if st.focusSrc {
		st.focusSrc = false
		f.Focus()
	}
	if f.Changed() {
		st.token++
		token := st.token
		time.AfterFunc(searchPause, func() {
			m.o.Post(func() {
				if token == st.token {
					st.query = strings.TrimSpace(st.draft)
				}
			})
		})
	}
	if f.Submitted() {
		st.token++
		st.query = strings.TrimSpace(st.draft)
	}
}

func (m *Module) peopleList(c *ui.Context) {
	st := &m.people
	t := c.Theme()
	if st.list.loaded && len(st.rows) == 0 {
		text := "No Contacts"
		if st.query != "" {
			text = "No Results"
		}
		centered(c, text, 15)
		return
	}
	inert := st.editing != nil
	st.listUI.Selected = &st.row
	st.listUI.Key = func(i int) any {
		if i >= len(st.rows) {
			return i
		}
		if p := st.rows[i].person; p != nil {
			return p.ID
		}
		return "letter:" + st.rows[i].letter
	}
	st.listUI.Header = func(i int) bool { return i < len(st.rows) && st.rows[i].person == nil }
	st.listUI.Label = func(i int) string {
		if i < len(st.rows) && st.rows[i].person != nil {
			return personName(st.rows[i].person)
		}
		return ""
	}
	list := uikit.SoftList(c, &st.listUI, len(st.rows), func(i int) { m.personRow(c, st.rows[i], t) }).Label("Contacts").IgnoreInput(inert).Background(t.Background)
	if st.focusAsk == panePeople {
		st.focusAsk = ""
		list.Focus()
	}
	if list.FocusWithin() {
		st.focus = panePeople
	}
	if list.Changed() && st.row >= 0 && st.row < len(st.rows) && st.rows[st.row].person != nil {
		st.selected = st.rows[st.row].person.ID
	}
}

func personName(p *api.PersonSummary) string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return p.Email
}

func (m *Module) personRow(c *ui.Context, row peopleRow, t *ui.Theme) {
	if row.person == nil {
		ui.Row(c).Height(24).Padding(0, frostui.SpaceM).AlignItems(ui.Center).Background(frostui.Chrome(t)).Children(func() {
			frostui.Label(c, row.letter)
		})
		return
	}
	p := row.person
	name := personName(p)
	second := p.Organization
	if second == "" {
		second = p.Email
	}
	if second == name {
		second = ""
	}
	sep := t.Border
	ui.Row(c).Height(44).Padding(0, frostui.SpaceM).Gap(10).AlignItems(ui.Center).Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X + 50, Y: r.Y + r.H - 1, W: r.W - 50, H: 1}, sep, 0)
	}).Children(func() {
		avatar(c, 28, p.DisplayName, p.Email, nil)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			frostui.Strong(c, name).SingleLine()
			if second != "" {
				frostui.Muted(c, second).SingleLine()
			}
		})
	})
}

// personPane draws the selected person.
func (m *Module) personPane(c *ui.Context) {
	st := &m.people
	t := c.Theme()
	pane := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Focusable().TabStop(false).FocusRing(false).Background(t.Background).Label("Contact")
	if st.focusAsk == panePerson {
		st.focusAsk = ""
		pane.Focus()
	}
	if pane.FocusWithin() {
		st.focus = panePerson
	}
	pane.Children(func() {
		if e := st.editing; e != nil && (e.isNew || !m.showsContact(e.contact.ID)) {
			ui.Scroll(c).Grow(1).Fill().Children(func() {
				ui.Column(c).Padding(frostui.SpaceXL).Gap(20).AlignItems(ui.Stretch).Children(func() { e.view(c) })
			})
			return
		}
		p := m.shownPerson()
		if p == nil {
			centered(c, "No Contact Selected", 15)
			return
		}
		ui.Scroll(c).Grow(1).Fill().Children(func() {
			ui.Column(c).Padding(frostui.SpaceXL).Gap(20).AlignItems(ui.Stretch).Children(func() { m.personDetails(c, p) })
		})
	})
}

func (m *Module) personDetails(c *ui.Context, p *api.Person) {
	t := c.Theme()
	st := &m.people
	email := firstEmail(p)
	photo := m.photoOf(p)
	ui.Row(c).Gap(frostui.SpaceL).AlignItems(ui.Start).Children(func() {
		avatar(c, 64, p.DisplayName, email, photo)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(frostui.SpaceXS).AlignItems(ui.Start).Children(func() {
			name := p.DisplayName
			if name == "" {
				name = email
			}
			ui.Text(c, name).FontSize(frostui.TextSize(c, 20)).FontWeight(600).Wrap().Role(ui.RoleHeading).Selectable()
			var parts []string
			if len(p.Contacts) > 0 && p.Contacts[0].Title != "" {
				parts = append(parts, p.Contacts[0].Title)
			}
			if p.Organization != "" {
				parts = append(parts, p.Organization)
			}
			if len(parts) > 0 {
				frostui.Body(c, strings.Join(parts, " · ")).TextColor(t.TextMuted).Wrap().Selectable()
			}
			if frostui.LabeledButton(c, "mail-unread", "Message", true).Disabled(email == "").Clicked() && m.o.Compose != nil {
				m.o.Compose(email, p.DisplayName)
			}
		})
	})
	for i := range p.Contacts {
		m.contactBlock(c.Key(p.Contacts[i].ID), &p.Contacts[i])
	}
	if email != "" && st.card.value != nil && strings.EqualFold(st.card.value.Email, email) {
		m.personRecent(c, st.card.value)
	}
}

// photoOf is a person's photo, once loaded.
func (m *Module) photoOf(p *api.Person) *ui.Bitmap {
	if v := m.people.photo.value; p.HasPhoto && v.id == p.ID {
		return v.bitmap
	}
	return nil
}

func (m *Module) contactHeading(cc *api.Contact) string {
	name := ""
	if b := m.book(cc.CollectionID); b != nil {
		name = b.Name
	}
	text := name + " — " + m.accountEmail(cc.AccountID)
	if cc.ReadOnly {
		text += " · Read-only"
	}
	return text
}

func (m *Module) contactBlock(c *ui.Context, cc *api.Contact) {
	t := c.Theme()
	st := &m.people
	if e := st.editing; e != nil && !e.isNew && e.contact.ID == cc.ID {
		e.view(c)
		return
	}
	heading := m.contactHeading(cc)
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Role(ui.RoleRegion).Label(heading).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			sectionHeading(c, heading).Grow(1).Shrink(1).MinWidth(0)
			if !cc.ReadOnly && textButton(c, "Edit", t.Accent, 12).Label("Edit Contact").Disabled(st.editing != nil).Clicked() {
				m.editContact(cc)
			}
		})
		ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() { m.contactRows(c, cc) })
	})
}

func labelOr(label, fallback string) string {
	if label != "" {
		return label
	}
	return fallback
}

func (m *Module) contactRows(c *ui.Context, cc *api.Contact) {
	t := c.Theme()
	for i, v := range cc.Emails {
		row(c.Key([2]any{"email", i}), labelOr(v.Label, "email"), func() {
			if textButton(c, v.Value, t.Accent, 13).Clicked() && m.o.Compose != nil {
				m.o.Compose(v.Value, cc.DisplayName)
			}
		})
	}
	for i, v := range cc.Phones {
		row(c.Key([2]any{"phone", i}), labelOr(v.Label, "phone"), func() { frostui.Body(c, v.Value).Selectable() })
	}
	for i, a := range cc.Addresses {
		row(c.Key([2]any{"address", i}), labelOr(a.Label, "address"), func() {
			for j, line := range addressLines(a) {
				frostui.Body(c.Key(j), line).Selectable()
			}
		})
	}
	for i, v := range cc.URLs {
		row(c.Key([2]any{"url", i}), labelOr(v.Label, "homepage"), func() {
			if strings.HasPrefix(v.Value, "http://") || strings.HasPrefix(v.Value, "https://") {
				ui.Link(c, v.Value, v.Value).FontSize(frostui.TextSize(c, 13))
			} else {
				frostui.Body(c, v.Value).Selectable()
			}
		})
	}
	if cc.Birthday != "" {
		row(c.Key("birthday"), "birthday", func() { frostui.Body(c, birthdayText(cc.Birthday)) })
	}
	if cc.Nickname != "" {
		row(c.Key("nickname"), "nickname", func() { frostui.Body(c, cc.Nickname) })
	}
	if cc.Note != "" {
		row(c.Key("note"), "note", func() { frostui.Body(c, cc.Note).Wrap().Selectable() })
	}
}

func row(c *ui.Context, label string, value func()) { kvRow(c, label, 96, value) }

// addressLines are an address's non-empty lines.
func addressLines(a api.PostalAddress) []string {
	var lines []string
	for l := range strings.SplitSeq(a.Street, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	var place []string
	for _, p := range []string{a.Locality, a.Region, a.Postcode} {
		if p != "" {
			place = append(place, p)
		}
	}
	if len(place) > 0 {
		lines = append(lines, strings.Join(place, " "))
	}
	if a.Country != "" {
		lines = append(lines, a.Country)
	}
	return lines
}

// birthdayText writes a birthday: "December 10, 1815" or "December 9".
func birthdayText(b string) string {
	if rest, ok := strings.CutPrefix(b, "--"); ok {
		if t, err := time.Parse("01-02", rest); err == nil {
			return t.Format("January 2")
		}
		return b
	}
	if t, ok := parseDate(b); ok {
		return t.Format("January 2, 2006")
	}
	return b
}

func (m *Module) personRecent(c *ui.Context, card *api.ContactCard) {
	t := c.Theme()
	if len(card.Recent) > 0 {
		ui.Column(c.Key("recent")).AlignItems(ui.Stretch).Role(ui.RoleRegion).Label("Recent Mail").Children(func() {
			sectionHeading(c, "Recent Mail").Padding(0, 0, frostui.SpaceS, 0)
			for _, r := range card.Recent {
				subject := r.Subject
				label := subject
				color := t.Text
				if subject == "" {
					subject, label, color = "(No Subject)", "(No Subject)", tertiary(t)
				}
				b := ui.ButtonBase(c.Key(r.ID)).Row().Height(32).Gap(frostui.SpaceM).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label)
				if b.Clicked() && m.o.OpenMessage != nil {
					m.o.OpenMessage(r.ID)
				}
				b.Children(func() {
					frostui.Body(c, subject).TextColor(color).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					frostui.Muted(c, mail.ListDate(r.Date, m.now())).SingleLine()
				})
			}
		})
	}
	if len(card.Upcoming) > 0 {
		m.loadCalendars()
		ui.Column(c.Key("upcoming")).AlignItems(ui.Stretch).Role(ui.RoleRegion).Label("Upcoming").Children(func() {
			sectionHeading(c, "Upcoming").Padding(0, 0, frostui.SpaceS, 0)
			m.upcomingRows(c, card.Upcoming)
		})
	}
}

// upcomingRows draws occurrences as Upcoming's rows.
func (m *Module) upcomingRows(c *ui.Context, occs []api.Occurrence) {
	t := c.Theme()
	for i := range occs {
		o := occs[i]
		name := title(&o)
		when := m.clock.upcomingWhen(&o, m.now())
		b := ui.ButtonBase(c.Key(keyOf(&o))).Row().Height(32).Gap(frostui.SpaceM).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(name + ", " + when)
		if b.Clicked() {
			m.ShowOccurrence(o)
		}
		b.Children(func() {
			uikit.Dot(c, 6, m.colorOf(t, o.CalendarID))
			frostui.Body(c, name).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			frostui.Muted(c, when).SingleLine()
		})
	}
}
