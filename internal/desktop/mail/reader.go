package mail

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// InvitationPart is a message's iCalendar part: text/calendar,
// application/ics, or a file named *.ics; nil when it has none.
func InvitationPart(parts []api.Part) *api.Part {
	for i := range parts {
		if parts[i].ContentType == "text/calendar" || parts[i].ContentType == "application/ics" {
			return &parts[i]
		}
	}
	for i := range parts {
		if strings.HasSuffix(strings.ToLower(parts[i].Filename), ".ics") {
			return &parts[i]
		}
	}
	return nil
}

// Attachments are the parts shown as attachments rather than inline,
// without the invitation's part when the card shows it.
func Attachments(parts []api.Part, invitation bool) []api.Part {
	var skip *api.Part
	if invitation {
		skip = InvitationPart(parts)
	}
	var out []api.Part
	for i := range parts {
		p := parts[i]
		if skip != nil && skip.Path == p.Path {
			continue
		}
		if p.Disposition == "attachment" || (p.Filename != "" && p.Disposition != "inline") {
			out = append(out, p)
		}
	}
	return out
}

// InRole reports whether a message is in a mailbox of the role.
func InRole(s api.MessageSummary, role api.MailboxRole, mailboxes []api.Mailbox) bool {
	for _, m := range mailboxes {
		if m.Role == role && slices.Contains(s.MailboxIDs, m.ID) {
			return true
		}
	}
	return false
}

// FormatSize writes a size as "12 KB" or "1.4 MB".
func FormatSize(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%d KB", (n+500)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1e6)
}

// SaveName is a message's default file name: the subject, slashes made
// dashes, "(no subject)" when blank, and .eml.
func SaveName(subject string) string {
	name := strings.NewReplacer("/", "-", `\`, "-").Replace(strings.TrimSpace(subject))
	if name == "" {
		name = "(no subject)"
	}
	return name + ".eml"
}

// SenderCheckText is the sender check banner's text (ADR-0050).
func SenderCheckText(address string) string {
	domain := ""
	if at := strings.LastIndex(address, "@"); at >= 0 {
		domain = strings.ToLower(strings.TrimSpace(address[at+1:]))
	}
	if domain == "" {
		return "Sender not verified. Its sender did not pass the provider's checks. Be careful with its links and attachments."
	}
	return fmt.Sprintf("Sender not verified. This message says it is from %s, but %s did not vouch for it. Be careful with its links and attachments.", domain, domain)
}

// WhenText writes a time as the reminder banner shows it: "Today at 3:04
// PM", "Tomorrow at…", else the date.
func WhenText(t, now time.Time) string {
	t = t.In(now.Location())
	clock := t.Format("3:04 PM")
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()); {
	case day.Equal(today):
		return "Today at " + clock
	case day.Equal(today.AddDate(0, 0, 1)):
		return "Tomorrow at " + clock
	}
	return t.Format("Jan 2") + " at " + clock
}

// UnsubscribeMethods are the message's methods this account can use: a
// read-only account cannot send mail.
func UnsubscribeMethods(info *api.Unsubscribe, readOnly bool) []api.UnsubscribeMethod {
	var out []api.UnsubscribeMethod
	for _, m := range info.Methods {
		if m != api.UnsubscribeMethodMail || !readOnly {
			out = append(out, m)
		}
	}
	return out
}

// UnsubscribeSentence says what a method will do.
func UnsubscribeSentence(info *api.Unsubscribe, m api.UnsubscribeMethod) string {
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	switch m {
	case api.UnsubscribeMethodOneclick:
		return fmt.Sprintf("Frostmail will ask %s to take you off the list.", deref(info.Host))
	case api.UnsubscribeMethodMail:
		return fmt.Sprintf("Frostmail will send a message to %s asking to take you off the list.", deref(info.Address))
	}
	host := deref(info.URL)
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Host
	}
	return fmt.Sprintf("The list's page on %s will open in your browser.", host)
}

// Answer is one of an invitation's buttons.
type Answer struct {
	Answer api.PartStat
	Label  string
}

// Answers are the invitation's buttons, in order.
var Answers = []Answer{{api.PartStatAccepted, "Accept"}, {api.PartStatTentative, "Maybe"}, {api.PartStatDeclined, "Decline"}}

// InvitationText is what an invitation card says.
type InvitationText struct {
	Month, Day, Badge, When, Who, Conflicts, Adjacent, Status string
}

func answerWords(a api.PartStat) string {
	switch a {
	case api.PartStatAccepted:
		return "accepted"
	case api.PartStatDeclined:
		return "declined"
	case api.PartStatTentative:
		return "said maybe"
	}
	return ""
}

func titled(s string) string {
	if strings.TrimSpace(s) == "" {
		return "No Title"
	}
	return s
}

// InvitationLines words an invitation (docs/specs/pim-ui.md, Invitation
// card), in the zone given.
func InvitationLines(inv *api.Invitation, zone *time.Location) InvitationText {
	e := inv.Event
	start := e.Start.In(zone)
	var out InvitationText
	if e.AllDay {
		if d, err := time.Parse("2006-01-02", e.StartDate); err == nil {
			start = d
		}
		out.When = start.Format("Monday, January 2, 2006") + " · All day"
	} else {
		out.When = start.Format("Monday, January 2, 2006") + " · " + start.Format("3:04 PM") + " – " + e.End.In(zone).Format("3:04 PM")
	}
	out.Month, out.Day = strings.ToUpper(start.Format("Jan")), start.Format("2")
	switch inv.Method {
	case api.ITIPMethodRequest:
		out.Badge = "Invitation"
	case api.ITIPMethodCancel:
		out.Badge = "Cancelled"
	case api.ITIPMethodReply:
		out.Badge = "Reply"
	default:
		out.Badge = "Event"
	}
	person := func(a *api.Attendee) string {
		if a.Name != "" {
			return a.Name
		}
		return a.Email
	}
	if inv.Method == api.ITIPMethodReply {
		if inv.From != nil {
			out.Who = strings.TrimSpace(person(inv.From) + " " + answerWords(inv.From.Answer))
		}
	} else {
		var who []string
		if e.Location != "" {
			who = append(who, e.Location)
		}
		if e.Organizer != nil {
			who = append(who, person(e.Organizer)+" (organizer)")
		}
		out.Who = strings.Join(who, " · ")
	}
	if len(inv.Conflicts) > 0 {
		var names []string
		for _, o := range inv.Conflicts {
			names = append(names, titled(o.Summary))
		}
		out.Conflicts = "Conflicts with " + strings.Join(names, ", ")
	}
	var adjacent []string
	for i, o := range inv.Adjacent {
		word := "After"
		if i == 0 && !o.End.After(e.Start) {
			word = "Before"
		}
		adjacent = append(adjacent, word+": "+titled(o.Summary))
	}
	out.Adjacent = strings.Join(adjacent, " · ")
	switch {
	case inv.Outdated:
		out.Status = "This invitation is out of date."
	case inv.Answer != nil && answerWords(*inv.Answer) != "":
		out.Status = "You " + answerWords(*inv.Answer)
	}
	return out
}

// attachmentExtensions are the kinds of attachment the app opens with the
// system's handler: documents, pictures, sound, video, archives and the
// like. Anything else, which includes every type that runs or redirects
// and types a browser would run (HTML, SVG), is refused: "Save As…" keeps
// it instead.
var attachmentExtensions = []string{
	".pdf", ".txt", ".log", ".csv", ".tsv", ".md", ".rtf",
	".doc", ".docx", ".odt", ".xls", ".xlsx", ".ods", ".ppt", ".pptx", ".odp", ".pages", ".numbers", ".key", ".epub",
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff", ".heic", ".heif", ".avif",
	".mp3", ".m4a", ".aac", ".wav", ".ogg", ".oga", ".opus", ".flac",
	".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".ogv",
	".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar",
	".ics", ".vcf", ".eml",
}

// AttachmentOpener opens attachments under root. launch stands in for the
// system's handler in a test; nil is the system's.
func AttachmentOpener(root string, launch func(path string) error) frostui.Opener {
	return frostui.Opener{Roots: []string{root}, Extensions: attachmentExtensions, LaunchFile: launch}
}
