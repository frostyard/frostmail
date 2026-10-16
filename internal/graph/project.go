package graph

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// Projections read a source back into Graph's fields (ADR-0040;
// docs/design/pim.md, Microsoft Graph). A write compares the projection
// of the source derived from Graph's JSON with the projection of the
// source as edited here and sends only the fields that differ, so what
// Frostmail does not model (an HTML body, categories, a tentative
// showAs) is never written over.

// ErrUnsupported means a source holds what Outlook cannot store, such as
// a recurrence rule outside Graph's patterns.
var ErrUnsupported = errors.New("graph: Outlook cannot store this")

// EventProjection is an event or a series as Graph's fields.
type EventProjection struct {
	UID    string
	Master EventPart
	// Overrides are the series' changed occurrences, by
	// calendar.RecurrenceKey of their original start.
	Overrides map[string]EventPart
	// Cancelled are the series' deleted occurrences, by RecurrenceKey.
	Cancelled []string
	// Organizer is whether the user organizes the event: an event without
	// people is the user's own.
	Organizer bool
	AllDay    bool

	zone   string         // Graph's name of the series' zone
	loc    *time.Location // the zone wall times are written in
	master calendar.Event
}

// EventPart is one VEVENT: its fields but the recurrence's, and the
// user's answer (calendar.Event's PartStat) when invited.
type EventPart struct {
	Fields     map[string]any
	Answer     string
	Start, End time.Time
}

// ProjectEvent reads an iCalendar source into Graph's fields. userEmails
// are the account's addresses, lowercased; local is the zone floating
// times are read in and all-day events are written in.
func ProjectEvent(src []byte, userEmails []string, local *time.Location) (EventProjection, error) {
	events, err := calendar.Parse(src, calendar.Options{Local: local, UserEmails: userEmails})
	if err != nil {
		return EventProjection{}, err
	}
	out := EventProjection{Overrides: map[string]EventPart{}}
	var master *calendar.Event
	for i, e := range events {
		if e.RecurrenceID != "" {
			continue
		}
		if master != nil {
			return EventProjection{}, errors.New("graph: a source with two masters")
		}
		master = &events[i]
	}
	if master == nil {
		return EventProjection{}, errors.New("graph: a source without a master")
	}
	out.UID, out.AllDay, out.master = master.UID, master.AllDay, *master
	out.Organizer = master.Organizer == nil || slices.Contains(userEmails, master.Organizer.Email)
	out.zone, out.loc = zoneFor(master, local)
	for _, e := range events {
		part := EventPart{Fields: out.fields(e, userEmails), Answer: e.PartStat, Start: e.Start, End: e.End}
		if e.RecurrenceID != "" {
			out.Overrides[e.RecurrenceID] = part
			continue
		}
		out.Master = part
	}
	if master.Recurrence != "" {
		rec, cancelled, err := recurrenceOf(*master, out.zone, local)
		if err != nil {
			return EventProjection{}, err
		}
		if rec != nil {
			out.Master.Fields["recurrence"] = rec
		}
		out.Cancelled = cancelled
	}
	return out, nil
}

// zoneFor is the zone an event's times are written in, as Graph names it
// and as a location: an all-day event's in local, a zone Graph has no
// name for (a fixed offset) in UTC.
func zoneFor(e *calendar.Event, local *time.Location) (string, *time.Location) {
	name, loc := e.TZID, e.Zone
	if e.AllDay {
		name, loc = local.String(), local
	} else if e.Floating {
		name = local.String()
	}
	if zone := GraphZone(name); zone != "UTC" || name == "UTC" {
		return zone, loc
	}
	return "UTC", time.UTC
}

// GraphZone names an IANA zone as Graph takes it: by its Windows name
// when it has one, else by its IANA name; "UTC" for a zone without a
// name.
func GraphZone(name string) string {
	if name == "" || name == "Local" {
		return "UTC"
	}
	if w, ok := calendar.WindowsName(name); ok {
		return w
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "UTC"
	}
	return name
}

// when is a start or end in Graph's form: an all-day event's date at
// midnight, else the wall time in the event's zone.
func (p EventProjection) when(t time.Time) map[string]any {
	if p.AllDay {
		return map[string]any{"dateTime": t.UTC().Format(time.DateOnly) + "T00:00:00", "timeZone": p.zone}
	}
	return map[string]any{"dateTime": t.In(p.loc).Format("2006-01-02T15:04:05"), "timeZone": p.zone}
}

// fields are one VEVENT's Graph fields but its recurrence. Only the
// organizer's have attendees: an invitee's copy keeps the organizer's.
func (p EventProjection) fields(e calendar.Event, userEmails []string) map[string]any {
	f := map[string]any{
		"subject":  e.Summary,
		"location": map[string]any{"displayName": e.Location},
		"body":     map[string]any{"contentType": "text", "content": strings.TrimSpace(e.Description)},
		"isAllDay": e.AllDay,
		"start":    p.when(e.Start),
		"end":      p.when(e.End),
		"showAs":   "busy",
	}
	if e.Transparent {
		f["showAs"] = "free"
	}
	f["isReminderOn"] = false
	for _, a := range e.Alarms {
		if a.At.IsZero() && !a.RelatedEnd && a.Offset <= 0 {
			f["isReminderOn"] = true
			f["reminderMinutesBeforeStart"] = int(-a.Offset / time.Minute)
			break
		}
	}
	if !p.Organizer {
		return f
	}
	attendees := []any{}
	for _, a := range e.Attendees {
		if slices.Contains(userEmails, a.Email) {
			continue
		}
		kind := "required"
		switch a.Role {
		case "optional":
			kind = "optional"
		case "none":
			kind = "resource"
		}
		attendees = append(attendees, map[string]any{"type": kind,
			"emailAddress": map[string]any{"address": a.Email, "name": a.Name}})
	}
	f["attendees"] = attendees
	return f
}

// Occurrence is an occurrence of the series as the master makes it: the
// master's fields at the occurrence's start, the base an override's
// fields are compared with.
func (p EventProjection) Occurrence(key string) (map[string]any, error) {
	start, err := KeyTime(key)
	if err != nil {
		return nil, err
	}
	f := maps.Clone(p.Master.Fields)
	delete(f, "recurrence")
	f["start"], f["end"] = p.when(start), p.when(start.Add(p.Master.End.Sub(p.Master.Start)))
	return f, nil
}

// Generates reports whether the series makes an occurrence with the key,
// its overrides aside: one its rule makes and no EXDATE takes away.
func (p EventProjection) Generates(key string) bool {
	at, err := KeyTime(key)
	if err != nil || p.master.Recurrence == "" {
		return false
	}
	occ, err := calendar.Expand([]calendar.Event{p.master}, at.Add(-time.Second), at.Add(time.Second))
	return err == nil && slices.ContainsFunc(occ, func(o calendar.Occurrence) bool { return o.RecurrenceID == key })
}

// KeyTime reads a calendar.RecurrenceKey: a date's midnight UTC, or an
// instant.
func KeyTime(key string) (time.Time, error) {
	if len(key) == len(time.DateOnly) {
		return time.Parse(time.DateOnly, key)
	}
	return time.Parse(time.RFC3339Nano, key)
}

// recurrenceOf turns a master's RRULE into Graph's recurrence, and its
// EXDATEs into cancelled occurrences.
func recurrenceOf(e calendar.Event, zoneName string, local *time.Location) (map[string]any, []string, error) {
	zone := e.Zone
	if zone == nil {
		zone = time.UTC
	}
	var rule string
	var cancelled []string
	for line := range strings.SplitSeq(e.Recurrence, "\n") {
		name, value, ok := cutProperty(line)
		if !ok {
			continue
		}
		head, params, _ := strings.Cut(name, ";")
		switch strings.ToUpper(head) {
		case "RRULE":
			if rule != "" {
				return nil, nil, fmt.Errorf("%w: two RRULEs", ErrUnsupported)
			}
			rule = value
		case "RDATE":
			return nil, nil, fmt.Errorf("%w: RDATE", ErrUnsupported)
		case "EXDATE":
			loc := zone
			if tzid := paramValue(params, "TZID"); tzid != "" {
				loc = calendar.ZoneNamed(tzid, local)
			}
			for v := range strings.SplitSeq(value, ",") {
				t, date, err := parseICalTime(v, loc)
				if err != nil {
					return nil, nil, fmt.Errorf("graph: EXDATE %q: %w", v, err)
				}
				cancelled = append(cancelled, calendar.RecurrenceKey(t, date || e.AllDay))
			}
		}
	}
	if rule == "" {
		return nil, cancelled, nil
	}
	rec, err := RecurrenceFromRule(rule, e.Start, zone, zoneName, e.AllDay)
	return rec, cancelled, err
}

// cutProperty splits a content line at the colon after its name and
// parameters, past any quoted parameter value.
func cutProperty(line string) (string, string, bool) {
	quoted := false
	for i, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ':' && !quoted:
			return line[:i], line[i+1:], true
		}
	}
	return "", "", false
}

func paramValue(params, name string) string {
	for p := range strings.SplitSeq(params, ";") {
		k, v, ok := strings.Cut(p, "=")
		if ok && strings.EqualFold(k, name) {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// parseICalTime reads a DATE or DATE-TIME value; date reports a DATE.
func parseICalTime(v string, loc *time.Location) (t time.Time, date bool, err error) {
	v = strings.TrimSpace(v)
	switch {
	case len(v) == len("20060102"):
		t, err = time.Parse("20060102", v)
		return t, true, err
	case strings.HasSuffix(v, "Z"):
		t, err = time.Parse("20060102T150405Z", v)
	default:
		t, err = time.ParseInLocation("20060102T150405", v, loc)
	}
	return t, false, err
}

var graphDays = map[string]string{"SU": "sunday", "MO": "monday", "TU": "tuesday", "WE": "wednesday",
	"TH": "thursday", "FR": "friday", "SA": "saturday"}

var graphIndexes = map[string]string{"1": "first", "2": "second", "3": "third", "4": "fourth", "-1": "last"}

// ruleParts are the RRULE parts each frequency may have in a rule Graph
// can hold.
var ruleParts = map[string][]string{
	"DAILY":   {"BYDAY"},
	"WEEKLY":  {"BYDAY", "WKST"},
	"MONTHLY": {"BYDAY", "BYMONTHDAY", "BYSETPOS"},
	"YEARLY":  {"BYDAY", "BYMONTH", "BYMONTHDAY", "BYSETPOS"},
}

// RecurrenceFromRule is an RRULE value as Graph's recurrence, for a series
// starting at start in zone (named zoneName for Graph): the reverse of
// RRule. A rule outside Graph's patterns is ErrUnsupported.
func RecurrenceFromRule(rule string, start time.Time, zone *time.Location, zoneName string, allDay bool) (map[string]any, error) {
	parts := map[string]string{}
	for p := range strings.SplitSeq(rule, ";") {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("%w: RRULE part %q", ErrUnsupported, p)
		}
		parts[strings.ToUpper(k)] = strings.ToUpper(v)
	}
	freq := parts["FREQ"]
	allowed, ok := ruleParts[freq]
	if !ok {
		return nil, fmt.Errorf("%w: FREQ=%s", ErrUnsupported, freq)
	}
	for k := range parts {
		switch k {
		case "FREQ", "INTERVAL", "COUNT", "UNTIL":
		default:
			if !slices.Contains(allowed, k) {
				return nil, fmt.Errorf("%w: %s in a %s rule", ErrUnsupported, k, strings.ToLower(freq))
			}
		}
	}
	local := start.In(zone)
	if allDay {
		local = start.UTC()
	}
	interval := 1
	if v, ok := parts["INTERVAL"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%w: INTERVAL=%s", ErrUnsupported, v)
		}
		interval = n
	}
	pattern := map[string]any{"interval": interval}
	weekday := strings.ToLower(local.Weekday().String())
	switch freq {
	case "DAILY":
		pattern["type"] = "daily"
		if v := parts["BYDAY"]; v != "" {
			// Weekdays and the like: Graph's weekly pattern.
			d, index, err := ruleDays(v)
			if err != nil || index != "" || interval != 1 {
				return nil, fmt.Errorf("%w: a daily rule with BYDAY=%s", ErrUnsupported, v)
			}
			pattern["type"], pattern["daysOfWeek"], pattern["firstDayOfWeek"] = "weekly", d, "monday"
		}
	case "WEEKLY":
		pattern["type"], pattern["daysOfWeek"], pattern["firstDayOfWeek"] = "weekly", []any{weekday}, "monday"
		if v := parts["BYDAY"]; v != "" {
			d, index, err := ruleDays(v)
			if err != nil || index != "" {
				return nil, fmt.Errorf("%w: BYDAY=%s", ErrUnsupported, v)
			}
			pattern["daysOfWeek"] = d
		}
		if v := parts["WKST"]; v != "" {
			w, ok := graphDays[v]
			if !ok {
				return nil, fmt.Errorf("%w: WKST=%s", ErrUnsupported, v)
			}
			pattern["firstDayOfWeek"] = w
		}
	case "MONTHLY", "YEARLY":
		yearly := freq == "YEARLY"
		if yearly {
			month := int(local.Month())
			if v := parts["BYMONTH"]; v != "" {
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 12 {
					return nil, fmt.Errorf("%w: BYMONTH=%s", ErrUnsupported, v)
				}
				month = n
			}
			pattern["month"] = month
		}
		if v := parts["BYDAY"]; v != "" {
			d, index, err := ruleDays(v)
			if err != nil {
				return nil, err
			}
			if pos := parts["BYSETPOS"]; pos != "" {
				if index != "" {
					return nil, fmt.Errorf("%w: BYDAY=%s with BYSETPOS", ErrUnsupported, v)
				}
				index = pos
			}
			name, ok := graphIndexes[cmpOr(index, "1")]
			if !ok || parts["BYMONTHDAY"] != "" {
				return nil, fmt.Errorf("%w: BYDAY=%s", ErrUnsupported, v)
			}
			pattern["type"] = "relativeMonthly"
			if yearly {
				pattern["type"] = "relativeYearly"
			}
			pattern["daysOfWeek"], pattern["index"] = d, name
			break
		}
		if parts["BYSETPOS"] != "" {
			return nil, fmt.Errorf("%w: BYSETPOS without BYDAY", ErrUnsupported)
		}
		day := local.Day()
		if v := parts["BYMONTHDAY"]; v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 31 {
				return nil, fmt.Errorf("%w: BYMONTHDAY=%s", ErrUnsupported, v)
			}
			day = n
		}
		pattern["type"] = "absoluteMonthly"
		if yearly {
			pattern["type"] = "absoluteYearly"
		}
		pattern["dayOfMonth"] = day
	}
	rng := map[string]any{"type": "noEnd", "startDate": local.Format(time.DateOnly), "recurrenceTimeZone": zoneName}
	switch {
	case parts["COUNT"] != "" && parts["UNTIL"] != "":
		return nil, fmt.Errorf("%w: COUNT with UNTIL", ErrUnsupported)
	case parts["COUNT"] != "":
		n, err := strconv.Atoi(parts["COUNT"])
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%w: COUNT=%s", ErrUnsupported, parts["COUNT"])
		}
		rng["type"], rng["numberOfOccurrences"] = "numbered", n
	case parts["UNTIL"] != "":
		until, date, err := parseICalTime(parts["UNTIL"], zone)
		if err != nil {
			return nil, fmt.Errorf("%w: UNTIL=%s", ErrUnsupported, parts["UNTIL"])
		}
		// Graph ends on a day: the last one with an occurrence by UNTIL,
		// which is the day before UNTIL's when UNTIL comes before the
		// series' time of day.
		end := until.In(zone)
		switch {
		case date || allDay:
			end = until.UTC()
		case clock(end) < clock(local):
			end = end.AddDate(0, 0, -1)
		}
		rng["type"], rng["endDate"] = "endDate", end.Format(time.DateOnly)
	}
	return map[string]any{"pattern": pattern, "range": rng}, nil
}

// clock is a wall time's time of day.
func clock(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
}

// ruleDays reads BYDAY: Graph's day names, and the one index they share
// ("" for none).
func ruleDays(byday string) ([]any, string, error) {
	var out []any
	index := ""
	for i, d := range strings.Split(byday, ",") {
		n := strings.TrimRight(d, "MOTUWEHFRSA")
		name, ok := graphDays[d[len(n):]]
		if !ok || i > 0 && n != index {
			return nil, "", fmt.Errorf("%w: BYDAY=%s", ErrUnsupported, byday)
		}
		index = n
		out = append(out, name)
	}
	return out, strings.TrimPrefix(index, "+"), nil
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// Diff is the fields of next whose JSON differs from base's.
func Diff(base, next map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range next {
		if !sameJSON(base[k], v) {
			out[k] = v
		}
	}
	return out
}

func sameJSON(a, b any) bool {
	ja, err1 := json.Marshal(a, json.Deterministic(true))
	jb, err2 := json.Marshal(b, json.Deterministic(true))
	return err1 == nil && err2 == nil && bytes.Equal(ja, jb)
}

// ContactProjection is a contact as Graph's fields, and its photo.
type ContactProjection struct {
	Fields    map[string]any
	Photo     []byte
	PhotoType string
}

// ProjectContact reads a vCard into Graph's fields. Phones go to Graph's
// slots by label: the first mobile number to mobilePhone, home numbers to
// homePhones, the rest to businessPhones.
func ProjectContact(src []byte) (ContactProjection, error) {
	card, err := vcardx.Parse(src)
	if err != nil {
		return ContactProjection{}, err
	}
	f := map[string]any{
		"displayName": card.FormattedName, "givenName": card.GivenName, "surname": card.FamilyName,
		"middleName": card.MiddleName, "title": card.Prefix, "generation": card.Suffix, "nickName": card.Nickname,
		"companyName": card.Organization, "department": card.Department, "jobTitle": card.Title,
		"personalNotes": card.Note,
	}
	emails := []any{}
	for _, e := range card.Emails {
		emails = append(emails, map[string]any{"address": e.Value})
	}
	f["emailAddresses"] = emails
	mobile, business, home := "", []any{}, []any{}
	for _, p := range card.Phones {
		switch {
		case p.Label == "mobile" && mobile == "":
			mobile = p.Value
		case p.Label == "home":
			home = append(home, p.Value)
		default:
			business = append(business, p.Value)
		}
	}
	f["mobilePhone"], f["businessPhones"], f["homePhones"] = mobile, business, home
	slots := map[string]string{"home": "homeAddress", "work": "businessAddress"}
	for _, key := range []string{"homeAddress", "businessAddress", "otherAddress"} {
		f[key] = map[string]any{}
	}
	taken := map[string]bool{}
	for _, a := range card.Addresses {
		key := cmpOr(slots[a.Label], "otherAddress")
		if taken[key] {
			continue
		}
		taken[key] = true
		f[key] = map[string]any{"street": a.Street, "city": a.Locality, "state": a.Region,
			"postalCode": a.Postcode, "countryOrRegion": a.Country}
	}
	f["businessHomePage"] = ""
	if len(card.URLs) > 0 {
		f["businessHomePage"] = card.URLs[0].Value
	}
	f["birthday"] = nil
	switch day, err := time.Parse(time.DateOnly, card.Birthday); {
	case err == nil:
		// Outlook keeps a birthday at 11:59 UTC, the same day everywhere.
		f["birthday"] = day.Add(11*time.Hour + 59*time.Minute).Format(time.RFC3339)
	case card.Birthday != "":
		return ContactProjection{}, fmt.Errorf("%w: a birthday without its year", ErrUnsupported)
	}
	return ContactProjection{Fields: f, Photo: card.Photo.Data, PhotoType: card.Photo.Type}, nil
}
