package pim

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// contactEditor is the open editor of a contact block, or a new contact.
type contactEditor struct {
	m            *Module
	isNew        bool
	contact      *api.Contact
	collectionID int64
	draft        contactDraft
	photoChanged bool
	photo        *api.Photo // the new photo; nil removes it
	preview      *ui.Bitmap
	hadPhoto     bool
	busy         bool
	failure      string
	custom       map[string]bool
}

var monthNames = []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

func (m *Module) editContact(cc *api.Contact) {
	st := &m.people
	if st.editing != nil {
		return
	}
	p := m.shownPerson()
	st.editing = &contactEditor{m: m, contact: cc, draft: contactDraftOf(cc), collectionID: cc.CollectionID, custom: map[string]bool{}, hadPhoto: p != nil && p.HasPhoto}
	if p != nil {
		st.editing.preview = m.photoOf(p)
	}
}

// newContact clears the selection and shows an empty editor.
func (m *Module) newContact() {
	st := &m.people
	books := m.writableBooks()
	if st.editing != nil || len(books) == 0 {
		return
	}
	id := books[0].ID
	for _, b := range books {
		if b.IsDefault {
			id = b.ID
			break
		}
	}
	st.before, st.selected = st.selected, 0
	st.editing = &contactEditor{m: m, isNew: true, collectionID: id, custom: map[string]bool{}}
}

func (e *contactEditor) cancel() {
	st := &e.m.people
	if e.busy {
		return
	}
	st.editing = nil
	if e.isNew {
		st.selected = st.before
	}
}

// done writes the changes: the fields, then the photo.
func (e *contactEditor) done() {
	if e.busy || len(validateContact(e.draft, e.isNew)) > 0 {
		return
	}
	m := e.m
	c := m.client
	if c == nil {
		return
	}
	e.busy, e.failure = true, ""
	fields := contactChanges(e.contact, e.draft)
	isNew, collection, contact, photoChanged, photo := e.isNew, e.collectionID, e.contact, e.photoChanged, e.photo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		var restores []string
		var person int64
		contactID := int64(0)
		var err error
		if isNew {
			var w *api.ContactWrite
			w, err = c.People().Create(ctx, &api.PeopleCreateParams{CollectionID: &collection, Fields: fields})
			if err == nil {
				contactID, restores = w.ContactID, append(restores, w.RestoreID)
				if w.Person != nil {
					person = w.Person.ID
				}
			}
		} else {
			contactID = contact.ID
			if !emptyContactFields(fields) {
				var w *api.ContactWrite
				w, err = c.People().Update(ctx, &api.PeopleUpdateParams{ID: contact.ID, Fields: fields})
				if err == nil {
					restores = append(restores, w.RestoreID)
				}
			}
		}
		if err == nil && photoChanged {
			var w *api.ContactWrite
			w, err = c.People().SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: contactID, Photo: photo})
			if err == nil {
				restores = append(restores, w.RestoreID)
				if w.Person != nil && person == 0 {
					person = w.Person.ID
				}
			}
		}
		m.o.Post(func() { e.finished(restores, person, err) })
	}()
}

func (e *contactEditor) finished(restores []string, person int64, err error) {
	st := &e.m.people
	for _, r := range restores {
		st.undo = remember(st.undo, r)
	}
	e.busy = false
	if err != nil {
		e.failure = err.Error()
		return
	}
	st.editing = nil
	if e.isNew && person != 0 {
		st.selected = person
	}
	st.list.dirty, st.person.dirty, st.photo.dirty = true, true, true
}

func (e *contactEditor) delete() {
	m := e.m
	cc := e.contact
	bookName := ""
	if b := m.book(cc.CollectionID); b != nil {
		bookName = b.Name
	}
	body := fmt.Sprintf(`Delete "%s" from %s — %s? It is removed from every device that syncs this address book.`, cc.DisplayName, bookName, m.accountEmail(cc.AccountID))
	id := cc.ID
	m.o.Dialogs.Ask(frostui.Confirm{Title: "Delete Contact?", Body: body, Action: "Delete", Destructive: true, OnConfirm: func() {
		c := m.client
		if c == nil {
			return
		}
		e.busy = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
			defer cancel()
			w, err := c.People().Delete(ctx, &api.PeopleDeleteParams{ID: id})
			m.o.Post(func() {
				e.busy = false
				if err != nil {
					e.failure = err.Error()
					return
				}
				st := &m.people
				st.undo = remember(st.undo, w.RestoreID)
				st.editing = nil
				if w.Person == nil {
					st.selected = 0
				}
				st.list.dirty, st.person.dirty = true, true
			})
		}()
	}})
}

func (e *contactEditor) pickPhoto() {
	m := e.m
	if m.o.OpenImage == nil {
		return
	}
	go func() {
		path, err := m.o.OpenImage()
		if err != nil || path == "" {
			if err != nil {
				m.o.Post(func() { e.failure = err.Error() })
			}
			return
		}
		data, err := os.ReadFile(path)
		var photo *api.Photo
		if err == nil {
			photo, err = scalePhoto(data)
		}
		m.o.Post(func() {
			if err != nil {
				e.failure = err.Error()
				return
			}
			e.photoChanged, e.photo, e.preview = true, photo, decodePhoto(photo)
		})
	}()
}

func editLabel(c *ui.Context, label string, fn func()) { kvRow(c, label, 96, fn) }

// view draws the editor in its block.
func (e *contactEditor) view(c *ui.Context) {
	t := c.Theme()
	m := e.m
	heading := "New Contact"
	if !e.isNew {
		heading = m.contactHeading(e.contact)
	}
	errs := validateContact(e.draft, e.isNew)
	d := &e.draft
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Role(ui.RoleRegion).Label(heading).Children(func() {
		sectionHeading(c, heading)
		if books := m.writableBooks(); e.isNew && len(books) > 1 {
			e.bookSelect(c, books)
		}
		e.photoWell(c)
		field := func(key string, value *string) {
			editLabel(c.Key(key), key, func() { ui.TextInput(c.Key(key), value).Label(key).FontSize(13).Height(28) })
		}
		field("first", &d.givenName)
		field("last", &d.familyName)
		field("nickname", &d.nickname)
		field("company", &d.organization)
		field("job title", &d.title)
		errorLine(c, errs["name"])
		e.valueList(c, kindEmail, "email", &d.emails, errs)
		e.valueList(c, kindPhone, "phone", &d.phones, errs)
		e.addressList(c)
		e.valueList(c, kindURL, "URL", &d.urls, errs)
		e.birthday(c, errs["birthday"])
		editLabel(c.Key("note"), "note", func() { ui.TextArea(c.Key("note"), &d.note).Label("note").FontSize(13).MinHeight(80) })
		ui.Row(c).Padding(frostui.SpaceL, 0, 0, 0).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			if !e.isNew && textButton(c.Key("delete"), "Delete Contact", flagColor(t, 1), 13).Disabled(e.busy).Clicked() {
				e.delete()
			}
			ui.Spacer(c)
			if ui.Button(c, "Cancel").Disabled(e.busy).Clicked() {
				e.cancel()
			}
			if ui.PrimaryButton(c, "Done").Disabled(e.busy || len(errs) > 0).Clicked() {
				e.done()
			}
		})
		if e.failure != "" {
			ui.Row(c).Role(ui.RoleAlert).Children(func() { frostui.Problem(c, e.failure) })
		}
	})
}

func (e *contactEditor) bookSelect(c *ui.Context, books []api.Collection) {
	m := e.m
	names := make([]string, len(books))
	current := ""
	for i, b := range books {
		names[i] = b.Name + " — " + m.accountEmail(b.AccountID)
		if b.ID == e.collectionID {
			current = names[i]
		}
	}
	editLabel(c.Key("address book"), "address book", func() {
		if ui.Select(c.Key("book"), &current, names).Label("address book").Changed() {
			e.collectionID = books[slices.Index(names, current)].ID
		}
	})
}

func (e *contactEditor) photoWell(c *ui.Context) {
	t := c.Theme()
	name, email := "", ""
	if e.contact != nil {
		name = e.contact.DisplayName
		if len(e.contact.Emails) > 0 {
			email = e.contact.Emails[0].Value
		}
	}
	editLabel(c.Key("photo row"), "photo", func() {
		b := ui.ButtonBase(c.Key("photo")).Size(64, 64).Radius(32).Label("Change Photo").Cursor(ui.CursorPointer)
		if b.Clicked() {
			e.pickPhoto()
		}
		b.Children(func() { avatar(c, 64, name, email, e.preview) })
		if e.preview != nil && addButton(c.Key("remove-photo"), "Remove Photo", t.Accent).Clicked() {
			e.photoChanged, e.photo, e.preview = true, nil, nil
		}
	})
}

// labelSelect draws a row's label: a select, or the custom field.
func (e *contactEditor) labelSelect(c *ui.Context, kind listKind, name string, i int, label *string) {
	key := fmt.Sprint(kind, i)
	if e.custom[key] {
		ui.TextInput(c.Key("custom"), label).Label(fmt.Sprintf("Custom label for %s %d", name, i+1)).Width(80).FontSize(13).Height(28)
		return
	}
	options := slices.Clone(contactLabels[kind])
	if *label != "" && !slices.Contains(options, *label) {
		options = append(options, *label)
	}
	options = append(options, "Custom…")
	if *label == "" {
		*label = options[0]
	}
	current := *label
	if ui.Select(c.Key("label"), &current, options).Label(fmt.Sprintf("Label for %s %d", name, i+1)).Width(80).Changed() {
		if current == "Custom…" {
			e.custom[key] = true
			*label = ""
		} else {
			*label = current
		}
	}
}

// valueList draws a list of labeled values and its add button.
func (e *contactEditor) valueList(c *ui.Context, kind listKind, name string, rows *[]api.LabeledValue, errs map[string]string) {
	t := c.Theme()
	title := map[listKind]string{kindEmail: "Email", kindPhone: "Phone", kindURL: "URL"}[kind]
	removed := -1
	for i := range *rows {
		v := &(*rows)[i]
		editLabel(c.Key([2]any{kind, i}), name, func() {
			ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
				e.labelSelect(c, kind, name, i, &v.Label)
				in := ui.TextInput(c.Key("value"), &v.Value).Label(fmt.Sprintf("%s %d", title, i+1)).FontSize(13).Height(28).Grow(1).Shrink(1).MinWidth(0)
				if errs["emails."+strconv.Itoa(i)] != "" && kind == kindEmail {
					in.Border(1, flagColor(t, 1)).Invalid(true)
				}
				if frostui.IconButton(c.Key("remove"), "list-remove", fmt.Sprintf("Remove %s %d", name, i+1)).Clicked() {
					removed = i
				}
			})
			if kind == kindEmail {
				errorLine(c, errs["emails."+strconv.Itoa(i)])
			}
		})
	}
	if removed >= 0 {
		*rows = slices.Delete(*rows, removed, removed+1)
	}
	editLabel(c.Key([2]any{kind, "add"}), "", func() {
		if addButton(c, "add "+name, t.Accent).Clicked() {
			used := make([]string, len(*rows))
			for i, r := range *rows {
				used[i] = r.Label
			}
			*rows = append(*rows, api.LabeledValue{Label: nextLabel(kind, used)})
		}
	})
}

func (e *contactEditor) addressList(c *ui.Context) {
	t := c.Theme()
	d := &e.draft
	removed := -1
	for i := range d.addresses {
		a := &d.addresses[i]
		editLabel(c.Key([2]any{kindAddress, i}), "address", func() {
			ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Start).Children(func() {
				e.labelSelect(c, kindAddress, "address", i, &a.Label)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).AlignItems(ui.Stretch).Children(func() {
					ui.TextArea(c.Key("street"), &a.Street).Label(fmt.Sprintf("Street %d", i+1)).FontSize(13).Lines(2, 2)
					ui.TextInput(c.Key("city"), &a.Locality).Label(fmt.Sprintf("City %d", i+1)).FontSize(13).Height(28)
					ui.TextInput(c.Key("state"), &a.Region).Label(fmt.Sprintf("State %d", i+1)).FontSize(13).Height(28)
					ui.TextInput(c.Key("zip"), &a.Postcode).Label(fmt.Sprintf("ZIP %d", i+1)).FontSize(13).Height(28)
					ui.TextInput(c.Key("country"), &a.Country).Label(fmt.Sprintf("Country %d", i+1)).FontSize(13).Height(28)
				})
				if frostui.IconButton(c.Key("remove"), "list-remove", fmt.Sprintf("Remove address %d", i+1)).Clicked() {
					removed = i
				}
			})
		})
	}
	if removed >= 0 {
		d.addresses = slices.Delete(d.addresses, removed, removed+1)
	}
	editLabel(c.Key([2]any{kindAddress, "add"}), "", func() {
		if addButton(c, "add address", t.Accent).Clicked() {
			used := make([]string, len(d.addresses))
			for i, r := range d.addresses {
				used[i] = r.Label
			}
			d.addresses = append(d.addresses, api.PostalAddress{Label: nextLabel(kindAddress, used)})
		}
	})
}

func (e *contactEditor) birthday(c *ui.Context, problem string) {
	b := &e.draft.birthday
	days := make([]string, 32)
	for i := 1; i <= 31; i++ {
		days[i] = strconv.Itoa(i)
	}
	editLabel(c.Key("birthday"), "birthday", func() {
		ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			month, _ := strconv.Atoi(b.month)
			name := monthNames[min(month, 12)]
			if ui.Select(c.Key("month"), &name, monthNames).Label("Birthday month").Changed() {
				b.month = twoDigit(slices.Index(monthNames, name))
			}
			day, _ := strconv.Atoi(b.day)
			dayText := days[min(day, 31)]
			if ui.Select(c.Key("day"), &dayText, days).Label("Birthday day").Changed() {
				b.day = twoDigit(slices.Index(days, dayText))
			}
			ui.TextInput(c.Key("year"), &b.year).Label("Birthday year").Width(64).FontSize(13).Height(28)
		})
		errorLine(c, problem)
	})
}

func twoDigit(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%02d", n)
}
