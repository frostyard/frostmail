package graph

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/mimex"
)

// The derived sources (ADR-0040; docs/design/pim.md, Microsoft Graph):
// iCalendar for an event or a series, vCard 4.0 for a contact. They are
// deterministic, so the same JSON always gives the same bytes.

const eol = "\r\n"

// graphTime is Graph's dateTime, which has no zone of its own: Frostmail
// asks for UTC.
const graphTime = "2006-01-02T15:04:05.9999999"

func parseGraphTime(d DateTimeTimeZone) (time.Time, error) {
	t, err := time.Parse(graphTime, d.DateTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("graph: time %q: %w", d.DateTime, err)
	}
	if d.TimeZone != "" && !strings.EqualFold(d.TimeZone, "UTC") {
		loc, err := zoneLocation(d.TimeZone)
		if err != nil {
			return time.Time{}, err
		}
		t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
	}
	return t.UTC(), nil
}

// ianaZone is the first of names that names a zone, IANA or Windows, as
// IANA; "UTC" when none does (Graph's custom zones).
func ianaZone(names ...string) string {
	for _, n := range names {
		if n == "" {
			continue
		}
		if z, ok := calendar.WindowsZone(n); ok {
			return z
		}
		if _, err := time.LoadLocation(n); err == nil && n != "Local" {
			return n
		}
	}
	return "UTC"
}

func zoneLocation(name string) (*time.Location, error) {
	return time.LoadLocation(ianaZone(name))
}

// EventSource derives the iCalendar object of an event that does not
// recur, or of a series: its master and exceptions. userEmail is the
// account's address, whose answer is the event's responseStatus.
func EventSource(master Event, exceptions []Event, userEmail string) ([]byte, error) {
	zone := seriesZone(master)
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	start, err := parseGraphTime(master.Start)
	if err != nil {
		return nil, err
	}
	zones := []string{zone}
	for _, ex := range exceptions {
		if z := ianaZone(ex.OriginalStartTimeZone); !slices.Contains(zones, z) {
			zones = append(zones, z)
		}
	}
	var out strings.Builder
	write := func(props ...contentline.Prop) {
		for _, p := range props {
			out.WriteString(p.Encode(eol))
		}
	}
	write(prop("BEGIN", "VCALENDAR"), prop("VERSION", "2.0"), prop("PRODID", "-//Frostyard//Frostmail Graph//EN"))
	if !master.IsAllDay {
		for _, z := range zones {
			if z == "UTC" {
				continue
			}
			vtz, err := calendar.VTimezone(z, start, eol)
			if err != nil {
				return nil, err
			}
			out.WriteString(vtz)
		}
	}
	uid := SourceUID(master.ICalUID)
	if uid == "" {
		uid = master.ID
	}
	main, err := eventProps(master, uid, userEmail)
	if err != nil {
		return nil, err
	}
	if master.Recurrence != nil {
		rule, err := RRule(*master.Recurrence, start, loc, master.IsAllDay)
		if err != nil {
			return nil, err
		}
		main = append(main, contentline.Prop{Name: "RRULE", Value: rule})
		for _, id := range master.CancelledOccurrences {
			at, err := occurrenceStart(id, "", start, loc)
			if err != nil {
				return nil, err
			}
			main = append(main, instantProp("EXDATE", at, loc, master.IsAllDay))
		}
	}
	write(prop("BEGIN", "VEVENT"))
	write(main...)
	write(alarm(master)...)
	write(prop("END", "VEVENT"))
	for _, ex := range exceptions {
		at, err := exceptionStart(ex, start, loc, master.IsAllDay)
		if err != nil {
			return nil, err
		}
		// Outlook gives an exception an iCalUId of its own (the master's with
		// the day in it); in iCalendar an override has its series' UID.
		props, err := eventProps(ex, uid, userEmail)
		if err != nil {
			return nil, err
		}
		props = append(props, instantProp("RECURRENCE-ID", at, loc, master.IsAllDay))
		write(prop("BEGIN", "VEVENT"))
		write(props...)
		write(alarm(ex)...)
		write(prop("END", "VEVENT"))
	}
	write(prop("END", "VCALENDAR"))
	return []byte(out.String()), nil
}

// goidClass starts the iCalUId Outlook gives an event (Exchange's
// GlobalObjectId, hex). One that came from outside, as an invitation from
// Google does, carries the UID it came with after vCalUID.
const goidClass = "040000008200E00074C5B7101A82E008"

var vCalUID = []byte("vCal-Uid\x01\x00\x00\x00")

// SourceUID is the iCalendar UID of an event with Outlook's iCalUId: the
// UID an invitation came with, else the iCalUId itself.
func SourceUID(iCalUID string) string {
	b, err := hex.DecodeString(iCalUID)
	if err != nil || len(b) < 40 || !strings.EqualFold(iCalUID[:len(goidClass)], goidClass) {
		return iCalUID
	}
	uid, ok := bytes.CutPrefix(b[40:], vCalUID)
	uid = bytes.TrimRight(uid, "\x00")
	if !ok || len(uid) == 0 || !utf8.Valid(uid) {
		return iCalUID
	}
	return string(uid)
}

// OutlookUID is the iCalUId Outlook gives an invitation that came with
// uid: the class, an instance date, a time and reserved bytes left zero,
// and the data's size and the data.
func OutlookUID(uid string) string {
	class, _ := hex.DecodeString(goidClass)
	data := append(append(slices.Clone(vCalUID), uid...), 0)
	b := append(class, make([]byte, 20)...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	return strings.ToUpper(hex.EncodeToString(append(b, data...)))
}

func prop(name, value string) contentline.Prop { return contentline.Prop{Name: name, Value: value} }

func text(name, value string) contentline.Prop {
	return contentline.Prop{Name: name, Value: contentline.EscapeText(value)}
}

// instantProp is a DTSTART-like property: a date for all-day events, else
// the wall time in loc with its TZID, or UTC with Z.
func instantProp(name string, t time.Time, loc *time.Location, allDay bool) contentline.Prop {
	if allDay {
		return contentline.Prop{Name: name, Params: []contentline.Param{{Name: "VALUE", Values: []string{"DATE"}}},
			Value: t.Format("20060102")}
	}
	if loc == time.UTC || loc.String() == "UTC" {
		return contentline.Prop{Name: name, Value: t.UTC().Format("20060102T150405Z")}
	}
	return contentline.Prop{Name: name, Params: []contentline.Param{{Name: "TZID", Values: []string{loc.String()}}},
		Value: t.In(loc).Format("20060102T150405")}
}

// eventProps are a VEVENT's properties but its RRULE, EXDATE,
// RECURRENCE-ID and alarm, under the series' UID.
func eventProps(e Event, uid, userEmail string) ([]contentline.Prop, error) {
	start, err := parseGraphTime(e.Start)
	if err != nil {
		return nil, err
	}
	end, err := parseGraphTime(e.End)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(ianaZone(e.OriginalStartTimeZone))
	if err != nil {
		return nil, err
	}
	endLoc, err := time.LoadLocation(ianaZone(e.OriginalEndTimeZone, e.OriginalStartTimeZone))
	if err != nil {
		return nil, err
	}
	props := []contentline.Prop{text("UID", uid)}
	if stamp, ok := stampOf(e.LastModifiedDateTime); ok {
		props = append(props, prop("DTSTAMP", stamp), prop("LAST-MODIFIED", stamp))
	}
	if created, ok := stampOf(e.CreatedDateTime); ok {
		props = append(props, prop("CREATED", created))
	}
	if e.IsAllDay {
		// All-day events run from midnight to midnight in their own zone.
		props = append(props, instantProp("DTSTART", allDayDate(start, loc), time.UTC, true),
			instantProp("DTEND", allDayDate(end, endLoc), time.UTC, true))
	} else {
		props = append(props, instantProp("DTSTART", start, loc, false), instantProp("DTEND", end, endLoc, false))
	}
	if e.Subject != "" {
		props = append(props, text("SUMMARY", e.Subject))
	}
	if e.Location.DisplayName != "" {
		props = append(props, text("LOCATION", e.Location.DisplayName))
	}
	if d := eventDescription(e); d != "" {
		props = append(props, text("DESCRIPTION", d))
	}
	if len(e.Categories) > 0 {
		escaped := make([]string, len(e.Categories))
		for i, c := range e.Categories {
			escaped[i] = contentline.EscapeText(c)
		}
		props = append(props, prop("CATEGORIES", strings.Join(escaped, ",")))
	}
	switch e.Sensitivity {
	case "private":
		props = append(props, prop("CLASS", "PRIVATE"))
	case "confidential":
		props = append(props, prop("CLASS", "CONFIDENTIAL"))
	}
	if e.IsCancelled {
		props = append(props, prop("STATUS", "CANCELLED"))
	}
	transp := "OPAQUE"
	if e.ShowAs == "free" {
		transp = "TRANSPARENT"
	}
	props = append(props, prop("TRANSP", transp))
	if e.OnlineMeeting != nil && e.OnlineMeeting.JoinURL != "" {
		props = append(props, prop("URL", e.OnlineMeeting.JoinURL))
	}
	return append(props, people(e, userEmail)...), nil
}

// allDayDate is the day an all-day event's start or end falls on, as
// midnight UTC: Graph gives midnight in the event's zone, which a UTC
// time read in that zone gives back; a time already at midnight UTC is
// that day.
func allDayDate(t time.Time, loc *time.Location) time.Time {
	if u := t.UTC(); u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 {
		return u
	}
	d := t.In(loc)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// stampOf turns Graph's timestamp into iCalendar's UTC form.
func stampOf(s string) (string, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "", false
	}
	return t.UTC().Format("20060102T150405Z"), true
}

func eventDescription(e Event) string {
	content := e.Body.Content
	if strings.EqualFold(e.Body.ContentType, "html") {
		content = mimex.HTMLToText(content)
	}
	return strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
}

// people are ORGANIZER and ATTENDEE, for an event with invitees. The
// user's own line carries the event's responseStatus.
func people(e Event, userEmail string) []contentline.Prop {
	if len(e.Attendees) == 0 {
		return nil
	}
	var props []contentline.Prop
	if e.Organizer != nil && e.Organizer.EmailAddress.Address != "" {
		props = append(props, person("ORGANIZER", e.Organizer.EmailAddress, nil))
	}
	userSeen := false
	for _, a := range e.Attendees {
		response := a.Status.Response
		mine := userEmail != "" && strings.EqualFold(a.EmailAddress.Address, userEmail)
		if mine {
			userSeen = true
			if e.ResponseStatus.Response != "" {
				response = e.ResponseStatus.Response
			}
		}
		props = append(props, person("ATTENDEE", a.EmailAddress, attendeeParams(a.Type, response)))
	}
	if !userSeen && !e.IsOrganizer && userEmail != "" {
		props = append(props, person("ATTENDEE", EmailAddress{Address: userEmail}, attendeeParams("required", e.ResponseStatus.Response)))
	}
	return props
}

func person(name string, a EmailAddress, params []contentline.Param) contentline.Prop {
	if a.Name != "" {
		params = append([]contentline.Param{{Name: "CN", Values: []string{a.Name}}}, params...)
	}
	return contentline.Prop{Name: name, Params: params, Value: "mailto:" + a.Address}
}

func attendeeParams(kind, response string) []contentline.Param {
	role, cutype := "REQ-PARTICIPANT", ""
	switch kind {
	case "optional":
		role = "OPT-PARTICIPANT"
	case "resource":
		role, cutype = "NON-PARTICIPANT", "RESOURCE"
	}
	partstat := "NEEDS-ACTION"
	switch response {
	case "accepted", "organizer":
		partstat = "ACCEPTED"
	case "declined":
		partstat = "DECLINED"
	case "tentativelyAccepted":
		partstat = "TENTATIVE"
	}
	params := []contentline.Param{{Name: "ROLE", Values: []string{role}}, {Name: "PARTSTAT", Values: []string{partstat}}}
	if cutype != "" {
		params = append(params, contentline.Param{Name: "CUTYPE", Values: []string{cutype}})
	}
	return params
}

// alarm is the event's reminder as a VALARM, when it has one on.
func alarm(e Event) []contentline.Prop {
	if !e.IsReminderOn {
		return nil
	}
	return []contentline.Prop{
		prop("BEGIN", "VALARM"), prop("ACTION", "DISPLAY"), text("DESCRIPTION", "Reminder"),
		prop("TRIGGER", "-PT"+strconv.Itoa(max(e.ReminderMinutesBeforeStart, 0))+"M"), prop("END", "VALARM"),
	}
}

// seriesZone is the IANA zone of an event or a series.
func seriesZone(master Event) string {
	if master.Recurrence != nil {
		return ianaZone(master.Recurrence.Range.RecurrenceTimeZone, master.OriginalStartTimeZone)
	}
	return ianaZone(master.OriginalStartTimeZone)
}

// exceptionStart is the original start of a series' exception, an
// all-day series' as its day.
func exceptionStart(ex Event, seriesStart time.Time, loc *time.Location, allDay bool) (time.Time, error) {
	at, err := occurrenceStart(ex.OccurrenceID, ex.OriginalStart, seriesStart, loc)
	if err == nil && allDay {
		at = allDayDate(at, loc)
	}
	return at, err
}

// occurrenceStart is the start of the occurrence an occurrence ID names
// ("OID.<master>.<YYYY-MM-DD>", the day in the series' zone): that day at
// the series' start time. A given original start wins.
func occurrenceStart(occurrenceID, original string, seriesStart time.Time, loc *time.Location) (time.Time, error) {
	if original != "" {
		if t, err := time.Parse(time.RFC3339Nano, original); err == nil {
			return t, nil
		}
	}
	dot := strings.LastIndexByte(occurrenceID, '.')
	if dot < 0 {
		return time.Time{}, fmt.Errorf("graph: occurrence ID %q has no day", occurrenceID)
	}
	day, err := time.ParseInLocation(time.DateOnly, occurrenceID[dot+1:], loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("graph: occurrence ID %q: %w", occurrenceID, err)
	}
	s := seriesStart.In(loc)
	return time.Date(day.Year(), day.Month(), day.Day(), s.Hour(), s.Minute(), s.Second(), 0, loc), nil
}

var weekdays = map[string]string{
	"sunday": "SU", "monday": "MO", "tuesday": "TU", "wednesday": "WE", "thursday": "TH", "friday": "FR", "saturday": "SA",
}

var indexes = map[string]string{"first": "1", "second": "2", "third": "3", "fourth": "4", "last": "-1"}

// RRule is a series' pattern and range as an RRULE value. An end date is
// the last day an occurrence may fall on, in the series' zone.
func RRule(r Recurrence, start time.Time, loc *time.Location, allDay bool) (string, error) {
	p := r.Pattern
	var parts []string
	days := func() (string, error) {
		var out []string
		for _, d := range p.DaysOfWeek {
			w, ok := weekdays[strings.ToLower(d)]
			if !ok {
				return "", fmt.Errorf("graph: day %q", d)
			}
			out = append(out, w)
		}
		if len(out) == 0 {
			return "", errors.New("graph: a pattern without days")
		}
		return strings.Join(out, ","), nil
	}
	relative := func() error {
		d, err := days()
		if err != nil {
			return err
		}
		index, ok := indexes[p.Index]
		if !ok {
			index = "1"
		}
		if len(p.DaysOfWeek) == 1 {
			parts = append(parts, "BYDAY="+index+d)
		} else {
			parts = append(parts, "BYDAY="+d, "BYSETPOS="+index)
		}
		return nil
	}
	switch p.Type {
	case "daily":
		parts = append(parts, "FREQ=DAILY")
	case "weekly":
		d, err := days()
		if err != nil {
			return "", err
		}
		parts = append(parts, "FREQ=WEEKLY", "BYDAY="+d)
		if w, ok := weekdays[strings.ToLower(p.FirstDayOfWeek)]; ok && w != "MO" {
			parts = append(parts, "WKST="+w)
		}
	case "absoluteMonthly":
		parts = append(parts, "FREQ=MONTHLY", "BYMONTHDAY="+strconv.Itoa(p.DayOfMonth))
	case "relativeMonthly":
		parts = append(parts, "FREQ=MONTHLY")
		if err := relative(); err != nil {
			return "", err
		}
	case "absoluteYearly":
		parts = append(parts, "FREQ=YEARLY", "BYMONTH="+strconv.Itoa(p.Month), "BYMONTHDAY="+strconv.Itoa(p.DayOfMonth))
	case "relativeYearly":
		parts = append(parts, "FREQ=YEARLY", "BYMONTH="+strconv.Itoa(p.Month))
		if err := relative(); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("graph: pattern %q", p.Type)
	}
	if p.Interval > 1 {
		parts = append(parts[:1], append([]string{"INTERVAL=" + strconv.Itoa(p.Interval)}, parts[1:]...)...)
	}
	switch r.Range.Type {
	case "numbered":
		parts = append(parts, "COUNT="+strconv.Itoa(r.Range.NumberOfOccurrences))
	case "endDate":
		end, err := time.ParseInLocation(time.DateOnly, r.Range.EndDate, loc)
		if err != nil {
			return "", fmt.Errorf("graph: end date %q: %w", r.Range.EndDate, err)
		}
		if allDay {
			parts = append(parts, "UNTIL="+end.Format("20060102"))
		} else {
			s := start.In(loc)
			last := time.Date(end.Year(), end.Month(), end.Day(), s.Hour(), s.Minute(), s.Second(), 0, loc)
			parts = append(parts, "UNTIL="+last.UTC().Format("20060102T150405Z"))
		}
	}
	return strings.Join(parts, ";"), nil
}

// ContactSource derives a contact's vCard 4.0; photo is its JPEG (or
// other image), nil for none.
func ContactSource(c Contact, photo []byte, photoType string) []byte {
	var out strings.Builder
	write := func(props ...contentline.Prop) {
		for _, p := range props {
			out.WriteString(p.Encode(eol))
		}
	}
	write(prop("BEGIN", "VCARD"), prop("VERSION", "4.0"), text("UID", c.ID))
	name := c.DisplayName
	if strings.TrimSpace(name) == "" {
		name = strings.Join(slices.DeleteFunc([]string{c.Title, c.GivenName, c.MiddleName, c.Surname, c.Generation},
			func(s string) bool { return s == "" }), " ")
	}
	if name == "" && len(c.EmailAddresses) > 0 {
		name = c.EmailAddresses[0].Address
	}
	write(text("FN", name))
	write(prop("N", contentline.JoinFields(c.Surname, c.GivenName, c.MiddleName, c.Title, c.Generation)))
	if c.NickName != "" {
		write(text("NICKNAME", c.NickName))
	}
	if c.CompanyName != "" || c.Department != "" {
		org := contentline.EscapeText(c.CompanyName)
		if c.Department != "" {
			org += ";" + contentline.EscapeText(c.Department)
		}
		write(prop("ORG", org))
	}
	if c.JobTitle != "" {
		write(text("TITLE", c.JobTitle))
	}
	for _, e := range c.EmailAddresses {
		if e.Address != "" {
			write(text("EMAIL", e.Address))
		}
	}
	tel := func(kind, number string) {
		if number != "" {
			write(contentline.Prop{Name: "TEL", Params: []contentline.Param{{Name: "TYPE", Values: []string{kind}}},
				Value: contentline.EscapeText(number)})
		}
	}
	tel("cell", c.MobilePhone)
	for _, n := range c.BusinessPhones {
		tel("work", n)
	}
	for _, n := range c.HomePhones {
		tel("home", n)
	}
	adr := func(kind string, a *PhysicalAddress) {
		if a == nil || *a == (PhysicalAddress{}) {
			return
		}
		var params []contentline.Param
		if kind != "" {
			params = []contentline.Param{{Name: "TYPE", Values: []string{kind}}}
		}
		write(contentline.Prop{Name: "ADR", Params: params,
			Value: contentline.JoinFields("", "", a.Street, a.City, a.State, a.PostalCode, a.CountryOrRegion)})
	}
	adr("home", c.HomeAddress)
	adr("work", c.BusinessAddress)
	adr("", c.OtherAddress)
	for _, im := range c.IMAddresses {
		if im != "" {
			write(text("IMPP", im))
		}
	}
	if c.BusinessHomePage != "" {
		write(prop("URL", c.BusinessHomePage))
	}
	if t, err := time.Parse(time.RFC3339Nano, c.Birthday); err == nil {
		write(prop("BDAY", t.UTC().Format("20060102")))
	}
	if c.PersonalNotes != "" {
		write(text("NOTE", strings.ReplaceAll(c.PersonalNotes, "\r\n", "\n")))
	}
	if len(c.Categories) > 0 {
		escaped := make([]string, len(c.Categories))
		for i, v := range c.Categories {
			escaped[i] = contentline.EscapeText(v)
		}
		write(prop("CATEGORIES", strings.Join(escaped, ",")))
	}
	if len(photo) > 0 {
		if photoType == "" {
			photoType = "image/jpeg"
		}
		write(prop("PHOTO", "data:"+photoType+";base64,"+base64.StdEncoding.EncodeToString(photo)))
	}
	if rev, ok := stampOf(c.LastModified); ok {
		write(prop("REV", rev))
	}
	write(prop("END", "VCARD"))
	return []byte(out.String())
}
