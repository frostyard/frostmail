package notify

// Reminder is what a reminder's notification shows (docs/design/pim.md,
// Reminders).
type Reminder struct {
	ID      string // calendar.reminders' id
	Summary string // the event's title
	Body    string // its time and place
}

// Reminder actions, besides "default" (open the app).
const (
	ActionSnooze  = "snooze"
	ActionDismiss = "dismiss"
)
