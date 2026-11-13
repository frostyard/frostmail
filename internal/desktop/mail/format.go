package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// ListDate shows a message list date by the local calendar days between it
// and now: the time for the same day, "Yesterday" for the day before, the
// weekday for 2 to 6 days before, and the short date otherwise.
func ListDate(d, now time.Time) string {
	d = d.In(now.Location())
	midnight := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()) }
	days := int(midnight(now).Sub(midnight(d)).Round(24*time.Hour) / (24 * time.Hour))
	switch {
	case days == 0:
		return d.Format("3:04 PM")
	case days == 1:
		return "Yesterday"
	case days >= 2 && days <= 6:
		return d.Format("Monday")
	}
	return d.Format("1/2/06")
}

// DisplayName is what the UI shows for an address.
func DisplayName(a api.Address) string {
	if name := strings.TrimSpace(a.Name); name != "" {
		return name
	}
	if address := strings.TrimSpace(a.Address); address != "" {
		return address
	}
	return "Unknown Sender"
}

// Coverage says how far back the searched accounts' stored mail goes, or
// "" when every one keeps every message (ADR-0016).
func Coverage(accounts []api.Account, now time.Time) string {
	var windowed []api.Account
	keepsAll := false
	for _, a := range accounts {
		if a.SyncDays > 0 {
			windowed = append(windowed, a)
		} else {
			keepsAll = true
		}
	}
	if len(windowed) == 0 {
		return ""
	}
	shared := true
	for _, a := range windowed {
		shared = shared && a.SyncDays == windowed[0].SyncDays
	}
	first := func(days int64) string {
		u := now.UTC()
		return time.Date(u.Year(), u.Month(), u.Day()-int(days), 0, 0, 0, 0, time.UTC).Format("January 2, 2006")
	}
	if shared && !keepsAll {
		return fmt.Sprintf("Searched mail since %s on this computer.", first(windowed[0].SyncDays))
	}
	var named []string
	for _, a := range windowed {
		name := strings.TrimSpace(a.DisplayName)
		if name == "" {
			name = a.Email
		}
		named = append(named, fmt.Sprintf("%s: since %s", name, first(a.SyncDays)))
	}
	return "Searched mail on this computer. " + strings.Join(named, "; ") + "."
}

// Choice is a Remind Me choice and its time.
type Choice struct {
	Label string
	At    time.Time
}

// RemindChoices offers an hour from now, tonight when future, and
// tomorrow at 08:00, in now's zone.
func RemindChoices(now time.Time) []Choice {
	y, m, d := now.Date()
	tonight := time.Date(y, m, d, 21, 0, 0, 0, now.Location())
	out := []Choice{{"Remind Me in 1 Hour", now.Add(time.Hour)}}
	if now.Before(tonight) {
		out = append(out, Choice{"Remind Me Tonight", tonight})
	}
	return append(out, Choice{"Remind Me Tomorrow", time.Date(y, m, d+1, 8, 0, 0, 0, now.Location())})
}

// OutboxStatus is an Outbox row's status line.
func OutboxStatus(it api.OutboxItem, now time.Time) string {
	switch it.State {
	case api.OutboxStateSending:
		return "Sending…"
	case api.OutboxStateAccepted:
		return "Saving to Sent…"
	case api.OutboxStateFailed:
		if it.Error != nil {
			return "Not sent: " + *it.Error
		}
		return "Not sent"
	}
	if it.Attempts == 0 {
		return "Waiting to send"
	}
	minutes := 1
	if it.SendAt != nil {
		minutes = max(1, int((it.SendAt.Sub(now)+time.Minute-1)/time.Minute))
	}
	if it.Error != nil {
		return fmt.Sprintf("Retrying in %d min: %s", minutes, *it.Error)
	}
	return fmt.Sprintf("Retrying in %d min", minutes)
}

// PreviewLines is how many lines of a message's preview the list shows:
// the user's choice, 0 to 5, else 2 (docs/specs/ui.md, Message list).
func PreviewLines(s *api.Settings) int {
	if s == nil || s.PreviewLines == nil || *s.PreviewLines < 0 || *s.PreviewLines > 5 {
		return 2
	}
	return int(*s.PreviewLines)
}
