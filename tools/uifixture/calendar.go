package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
)

// showcaseCalendar is a made-up calendar with its events.
type showcaseCalendar struct {
	name, color string
	readOnly    bool
	events      []showcaseEvent
}

// showcaseEvent is an event in floating local time, days after today
// (negative: before); end is minutes after start, or days for all-day ones.
type showcaseEvent struct {
	uid, summary, location string
	days                   int
	start                  string // "15:04", or "" for all day
	length                 int
	lines                  []string // more VEVENT lines: RRULE, ORGANIZER, ATTENDEE, STATUS, …
	alarm                  int      // minutes before; 0 for none
}

// showcaseCalendars are the showcase accounts' calendars around now: a
// working week with a daily standup, a review overlapping an interview, an
// unanswered invitation, a cancelled appointment and a multi-day offsite.
func showcaseCalendars(now time.Time) map[string][]showcaseCalendar {
	monday := -((int(now.Weekday()) + 6) % 7) - 28 // four weeks before this week's Monday
	ann, maria := "ann@northwind.example", "maria.lopez@northwind.example"
	who := func(role, email, name, partstat string) string {
		return fmt.Sprintf("%s;CN=%s;PARTSTAT=%s:mailto:%s", role, name, partstat, email)
	}
	return map[string][]showcaseCalendar{
		ann: {
			{name: "Work", color: "#3366cc", events: []showcaseEvent{
				{uid: "standup", summary: "Standup", location: "Zoom", days: monday, start: "09:30", length: 15, alarm: 10, lines: []string{
					"RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "ORGANIZER;CN=Maria Lopez:mailto:" + maria,
					who("ATTENDEE", ann, "Ann Patel", "ACCEPTED"), who("ATTENDEE", "victor.walker@northwind.example", "Victor Walker", "ACCEPTED"),
					who("ATTENDEE", "omar.ward@northwind.example", "Omar Ward", "TENTATIVE")}},
				{uid: "review", summary: "Design review", location: "Studio B", start: "14:00", length: 90, alarm: 15, lines: []string{
					"ORGANIZER;CN=Ann Patel:mailto:" + ann, who("ATTENDEE", "lena.price@northwind.example", "Lena Price", "ACCEPTED"),
					who("ATTENDEE", "grace.chen@northwind.example", "Grace Chen", "TENTATIVE"),
					`DESCRIPTION:Walk through the new calendar views.\nBring the latest screenshots.`}},
				{uid: "interview", summary: "Interview: Backend", location: "Room 2", start: "14:30", length: 45},
				{uid: "one-on-one", summary: "1:1 with Maria", days: monday + 1, start: "11:00", length: 30, lines: []string{"RRULE:FREQ=WEEKLY"}},
				{uid: "planning", summary: "Release planning", location: "Big room", days: 1, start: "10:00", length: 90, lines: []string{
					"ORGANIZER;CN=David King:mailto:david.king@northwind.example", who("ATTENDEE", ann, "Ann Patel", "NEEDS-ACTION")}},
				{uid: "retro", summary: "Incident retro", days: -1, start: "16:00", length: 60},
				{uid: "roadmap", summary: "Q4 roadmap", days: 2, start: "13:00", length: 60},
				{uid: "offsite", summary: "Company offsite", location: "Lisbon", days: 7 - (int(now.Weekday())+6)%7, length: 3},
			}},
			{name: "Holidays", color: "#ff9500", readOnly: true, events: []showcaseEvent{
				{uid: "holiday", summary: "Founders' Day", days: 3, length: 1, lines: []string{"TRANSP:TRANSPARENT"}},
			}},
		},
		"ann.lee@example.com": {
			{name: "Home", color: "#34c759", events: []showcaseEvent{
				{uid: "climbing", summary: "Climbing", location: "Riverside", days: monday + 2, start: "18:30", length: 90, lines: []string{"RRULE:FREQ=WEEKLY"}},
				{uid: "dentist", summary: "Dentist", days: 1, start: "08:00", length: 45, lines: []string{"STATUS:CANCELLED"}},
				{uid: "lunch", summary: "Lunch with Nina", location: "Cafe Nord", days: 2, start: "12:00", length: 60},
				{uid: "birthday", summary: "Nina's birthday", days: 2, length: 1},
			}},
		},
	}
}

// ics writes an event as a calendar object.
func (e showcaseEvent) ics(today time.Time) []byte {
	day := today.AddDate(0, 0, e.days)
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Frostmail//Showcase//EN", "BEGIN:VEVENT",
		"UID:" + e.uid + "@showcase", "DTSTAMP:20260101T000000Z", "SUMMARY:" + e.summary}
	if e.location != "" {
		lines = append(lines, "LOCATION:"+e.location)
	}
	if e.start == "" {
		lines = append(lines, "DTSTART;VALUE=DATE:"+day.Format("20060102"),
			"DTEND;VALUE=DATE:"+day.AddDate(0, 0, e.length).Format("20060102"))
	} else {
		start, _ := time.ParseInLocation("2006-01-02 15:04", day.Format("2006-01-02 ")+e.start, time.Local)
		lines = append(lines, "DTSTART:"+start.Format("20060102T150405"),
			"DTEND:"+start.Add(time.Duration(e.length)*time.Minute).Format("20060102T150405"))
	}
	lines = append(lines, e.lines...)
	if e.alarm > 0 {
		lines = append(lines, "BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:Reminder", fmt.Sprintf("TRIGGER:-PT%dM", e.alarm), "END:VALARM")
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}

// addShowcaseCalendars turns the calendar on for an account and stores its
// calendars' events, indexed and expanded as a pass of pimsync would.
func addShowcaseCalendars(ctx context.Context, db *store.DB, accountID int64, email string, now time.Time) error {
	cals := showcaseCalendars(now)[email]
	if len(cals) == 0 {
		return nil
	}
	y, m, d := now.In(time.Local).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	from, to := pimsync.Window(now)
	domain := email[strings.LastIndexByte(email, '@')+1:]
	opts := calendar.Options{Local: time.Local, UserEmails: []string{email}}
	return db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, accountID, api.ServiceKindCalendar, true, "https://dav."+domain+"/"); err != nil {
			return err
		}
		if err := tx.ServiceSynced(ctx, accountID, api.ServiceKindCalendar, now.Add(-4*time.Minute), ""); err != nil {
			return err
		}
		remote := make([]store.RemoteCollection, len(cals))
		for i, c := range cals {
			remote[i] = store.RemoteCollection{Href: "/calendars/" + strings.ToLower(c.name) + "/", Name: c.name, Color: c.color,
				ReadOnly: c.readOnly, Components: []string{"VEVENT"}}
		}
		cols, err := tx.ReplaceCollections(ctx, accountID, api.CollectionKindCalendar, remote)
		if err != nil {
			return err
		}
		for i, c := range cals {
			for _, e := range c.events {
				o := store.Object{CollectionID: cols[i].ID, Href: remote[i].Href + e.uid + ".ics", ETag: `"1"`, Raw: e.ics(today)}
				if _, err := pimsync.StoreCalendarObject(ctx, tx, o, opts, from, to); err != nil {
					return err
				}
			}
		}
		return tx.SetInstanceWindow(ctx, from, to)
	})
}
