package activity

import (
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// Names gives a client's names for the IDs in an entry: the account's name
// and the mailbox's, as the client already shows them; "" when unknown.
type Names interface {
	Account(id int64) string
	Mailbox(id int64) string
}

// Line writes an entry as a client's console shows it: its source (the
// account's name, "maild" for an entry of no account) and its text. Every
// word comes from this function or from names; the entry holds no text.
func Line(e api.ActivityEntry, names Names) (source, text string) {
	source = "maild"
	if e.AccountID != nil {
		source = name(names.Account(*e.AccountID), "Account %d", *e.AccountID)
	}
	mailbox := "a mailbox"
	if e.MailboxID != nil {
		mailbox = name(names.Mailbox(*e.MailboxID), "mailbox %d", *e.MailboxID)
	}
	srv := "the server"
	if e.Service != nil {
		srv = serverName(*e.Service)
	}
	switch e.Kind {
	case api.ActivityKindConnect:
		text = "connected to " + srv + took(e)
	case api.ActivityKindList:
		text = fmt.Sprintf("listed %s", plural(val(e.N), "mailbox", "mailboxes")) + took(e)
	case api.ActivityKindMailboxsynced:
		var parts []string
		if n := val(e.Added); n > 0 {
			parts = append(parts, fmt.Sprintf("%d new", n))
		}
		if n := val(e.Changed); n > 0 {
			parts = append(parts, fmt.Sprintf("%d changed", n))
		}
		if n := val(e.Removed); n > 0 {
			parts = append(parts, fmt.Sprintf("%d removed", n))
		}
		if len(parts) == 0 {
			text = mailbox + " synced, no changes" + took(e)
		} else {
			text = mailbox + " synced: " + strings.Join(parts, ", ") + took(e)
		}
	case api.ActivityKindIdle:
		text = mailbox + ": the server reported changes (IDLE)"
	case api.ActivityKindFetched:
		text = "fetched " + plural(val(e.N), "message body", "message bodies") + size(e.Bytes) + took(e)
	case api.ActivityKindPushed:
		text = "sent " + plural(val(e.N), "offline change", "offline changes") + " to the server"
	case api.ActivityKindDraftsaved:
		text = "saved " + plural(val(e.N), "draft", "drafts") + " on the server"
	case api.ActivityKindSent:
		text = "sent a message" + took(e)
	case api.ActivityKindSendfailed:
		text = "send failed: " + why(e)
		if e.Level == api.ActivityLevelWarn {
			text += "; retrying"
		}
	case api.ActivityKindSendlater:
		text = "scheduled " + plural(val(e.N), "message", "messages") + " to send later"
	case api.ActivityKindRulesapplied:
		text = "rules acted on " + plural(val(e.N), "message", "messages")
	case api.ActivityKindRemindfired:
		text = "Remind Me brought back " + plural(val(e.N), "message", "messages")
	case api.ActivityKindAnnounced:
		text = "announced " + plural(val(e.N), "new message", "new messages")
	case api.ActivityKindErased:
		text = mailbox + ": erased " + plural(val(e.N), "old message", "old messages")
	case api.ActivityKindPimsynced:
		if n := val(e.Changed); n > 0 {
			text = srv + " pass: changes in " + plural(n, "collection", "collections") + took(e)
		} else {
			text = srv + " pass, no changes" + took(e)
		}
	case api.ActivityKindPimrefused:
		text = srv + " refused a change: " + why(e)
	case api.ActivityKindTokenrefreshed:
		text = "refreshed the sign-in token"
	case api.ActivityKindSigninneeded:
		text = srv + " refused the sign-in (" + why(e) + "); sign in again"
	case api.ActivityKindOffline:
		text = srv + " cannot be reached (" + why(e) + "); retrying"
	case api.ActivityKindFailed:
		text = srv + " failed (" + why(e) + "); retrying"
	case api.ActivityKindDropped:
		text = plural(val(e.N), "entry", "entries") + " not shown, past the limit"
	default:
		text = string(e.Kind)
	}
	return source, text
}

func name(s, format string, id int64) string {
	if s != "" {
		return s
	}
	return fmt.Sprintf(format, id)
}

func serverName(s api.ActivityServer) string {
	switch s {
	case api.ActivityServerIMAP:
		return "IMAP"
	case api.ActivityServerSMTP:
		return "SMTP"
	case api.ActivityServerCaldav:
		return "CalDAV"
	case api.ActivityServerCarddav:
		return "CardDAV"
	case api.ActivityServerGraph:
		return "Microsoft Graph"
	case api.ActivityServerGtasks:
		return "Google Tasks"
	case api.ActivityServerOauth:
		return "the sign-in service"
	}
	return string(s)
}

// why is a failure's class and code, as "refused (550)".
func why(e api.ActivityEntry) string {
	s := "unknown"
	if e.ErrorClass != nil {
		s = string(*e.ErrorClass)
	}
	switch {
	case e.Code != nil:
		s += fmt.Sprintf(" (%d)", *e.Code)
	case e.IMAPCode != nil:
		s += " (" + strings.ToUpper(string(*e.IMAPCode)) + ")"
	}
	return s
}

func took(e api.ActivityEntry) string {
	if e.Ms == nil || *e.Ms <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%.1f s)", float64(*e.Ms)/1000)
}

func size(b *int64) string {
	if b == nil || *b <= 0 {
		return ""
	}
	switch n := float64(*b); {
	case n >= 1<<20:
		return fmt.Sprintf(", %.1f MB", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf(", %.0f KB", n/(1<<10))
	default:
		return fmt.Sprintf(", %d bytes", *b)
	}
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
