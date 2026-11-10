package pim

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// listKind is one of a contact's labeled lists.
type listKind string

const (
	kindEmail   listKind = "email"
	kindPhone   listKind = "phone"
	kindAddress listKind = "address"
	kindURL     listKind = "url"
)

// contactLabels are each kind's standard labels, in menu order.
var contactLabels = map[listKind][]string{
	kindEmail:   {"home", "work", "other"},
	kindPhone:   {"mobile", "home", "work", "main", "fax", "pager", "other"},
	kindAddress: {"home", "work", "other"},
	kindURL:     {"homepage", "home", "work", "other"},
}

// birthdayParts are a birthday's editable parts.
type birthdayParts struct{ month, day, year string }

// contactDraft is a contact's editable fields.
type contactDraft struct {
	givenName, familyName, nickname, organization, title string
	emails, phones, urls                                 []api.LabeledValue
	addresses                                            []api.PostalAddress
	birthday                                             birthdayParts
	note                                                 string
}

// contactDraftOf is an editable copy of a contact, or an empty new one.
func contactDraftOf(c *api.Contact) contactDraft {
	if c == nil {
		return contactDraft{}
	}
	d := contactDraft{givenName: c.GivenName, familyName: c.FamilyName, nickname: c.Nickname, organization: c.Organization, title: c.Title,
		emails: slices.Clone(c.Emails), phones: slices.Clone(c.Phones), urls: slices.Clone(c.URLs), addresses: slices.Clone(c.Addresses), note: c.Note}
	b := c.Birthday
	if s, ok := strings.CutPrefix(b, "--"); ok {
		if m, dd, ok := strings.Cut(s, "-"); ok {
			d.birthday = birthdayParts{month: m, day: dd}
		}
	} else if parts := strings.Split(b, "-"); len(parts) == 3 {
		d.birthday = birthdayParts{year: parts[0], month: parts[1], day: parts[2]}
	}
	return d
}

func birthdayValue(b birthdayParts) string {
	if b.month == "" && b.day == "" && b.year == "" {
		return ""
	}
	if b.year != "" {
		return b.year + "-" + b.month + "-" + b.day
	}
	return "--" + b.month + "-" + b.day
}

// nextLabel is the first standard label not used, or the last.
func nextLabel(kind listKind, used []string) string {
	labels := contactLabels[kind]
	for _, l := range labels {
		if !slices.Contains(used, l) {
			return l
		}
	}
	return labels[len(labels)-1]
}

var (
	twoDigits  = regexp.MustCompile(`^\d{2}$`)
	fourDigits = regexp.MustCompile(`^\d{4}$`)
	emailForm  = regexp.MustCompile(`^[^\s@]+@[^\s@]+$`)
)

func validBirthday(b birthdayParts) bool {
	if b.month == "" && b.day == "" && b.year == "" {
		return true
	}
	if !twoDigits.MatchString(b.month) || !twoDigits.MatchString(b.day) || (b.year != "" && !fourDigits.MatchString(b.year)) {
		return false
	}
	y := 2000
	if b.year != "" {
		y, _ = strconv.Atoi(b.year)
	}
	leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
	days := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if leap {
		days[1] = 29
	}
	m, _ := strconv.Atoi(b.month)
	d, _ := strconv.Atoi(b.day)
	return m >= 1 && m <= 12 && d >= 1 && d <= days[m-1]
}

// validateContact checks email syntax, real birthdays and a new contact's
// identity: field name to message.
func validateContact(d contactDraft, isNew bool) map[string]string {
	errs := map[string]string{}
	anyEmail := false
	for i, row := range d.emails {
		v := strings.TrimSpace(row.Value)
		anyEmail = anyEmail || v != ""
		if v != "" && !emailForm.MatchString(v) {
			errs["emails."+strconv.Itoa(i)] = "Enter an address like name@example.com."
		}
	}
	if !validBirthday(d.birthday) {
		errs["birthday"] = "This day doesn't exist."
	}
	if isNew && strings.TrimSpace(d.givenName+d.familyName+d.organization) == "" && !anyEmail {
		errs["name"] = "Add a name, a company or an email address."
	}
	return errs
}

func cleanValues(rows []api.LabeledValue) []api.LabeledValue {
	out := []api.LabeledValue{}
	for _, r := range rows {
		if v := strings.TrimSpace(r.Value); v != "" {
			out = append(out, api.LabeledValue{Label: strings.TrimSpace(r.Label), Value: v})
		}
	}
	return out
}

func cleanAddresses(rows []api.PostalAddress) []api.PostalAddress {
	out := []api.PostalAddress{}
	for _, r := range rows {
		a := api.PostalAddress{Label: strings.TrimSpace(r.Label), Street: strings.TrimSpace(r.Street), Locality: strings.TrimSpace(r.Locality),
			Region: strings.TrimSpace(r.Region), Postcode: strings.TrimSpace(r.Postcode), Country: strings.TrimSpace(r.Country)}
		if a.Street+a.Locality+a.Region+a.Postcode+a.Country != "" {
			out = append(out, a)
		}
	}
	return out
}

// contactChanges builds a trimmed patch; the note keeps its whitespace.
func contactChanges(c *api.Contact, d contactDraft) api.ContactFields {
	var f api.ContactFields
	text := func(have, draft string, trim bool, set func(*string)) {
		v := draft
		if trim {
			v = strings.TrimSpace(v)
		}
		if (c != nil && v != have) || (c == nil && v != "") {
			set(&v)
		}
	}
	var base api.Contact
	if c != nil {
		base = *c
	}
	text(base.GivenName, d.givenName, true, func(v *string) { f.GivenName = v })
	text(base.FamilyName, d.familyName, true, func(v *string) { f.FamilyName = v })
	text(base.Nickname, d.nickname, true, func(v *string) { f.Nickname = v })
	text(base.Organization, d.organization, true, func(v *string) { f.Organization = v })
	text(base.Title, d.title, true, func(v *string) { f.Title = v })
	text(base.Note, d.note, false, func(v *string) { f.Note = v })
	list := func(have, draft []api.LabeledValue, set func([]api.LabeledValue)) {
		rows := cleanValues(draft)
		if (c != nil && !slices.Equal(rows, have)) || (c == nil && len(rows) > 0) {
			set(rows)
		}
	}
	list(base.Emails, d.emails, func(v []api.LabeledValue) { f.Emails = v })
	list(base.Phones, d.phones, func(v []api.LabeledValue) { f.Phones = v })
	list(base.URLs, d.urls, func(v []api.LabeledValue) { f.URLs = v })
	if rows := cleanAddresses(d.addresses); (c != nil && !slices.Equal(rows, base.Addresses)) || (c == nil && len(rows) > 0) {
		f.Addresses = rows
	}
	if b := birthdayValue(d.birthday); (c != nil && b != base.Birthday) || (c == nil && b != "") {
		f.Birthday = &b
	}
	return f
}

func emptyContactFields(f api.ContactFields) bool {
	return f.GivenName == nil && f.FamilyName == nil && f.Nickname == nil && f.Organization == nil && f.Title == nil && f.Emails == nil &&
		f.Phones == nil && f.Addresses == nil && f.URLs == nil && f.Birthday == nil && f.Note == nil
}
