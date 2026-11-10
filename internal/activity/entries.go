package activity

import (
	"time"

	"github.com/frostyard/frostmail/api"
)

// The constructors: the only way to make an Entry. Each takes IDs, counts,
// durations and errors, never text; an error is reduced by Classify.

// Connected: logged in to the account's IMAP server, after d.
func Connected(account int64, d time.Duration) Entry {
	return entry(api.ActivityLevelDebug, api.ActivityKindConnect, account, func(e *api.ActivityEntry) {
		e.Service, e.Ms = server(api.ActivityServerIMAP), ms(d)
	})
}

// Listed: listed the account's n mailboxes, after d.
func Listed(account int64, n int, d time.Duration) Entry {
	return entry(api.ActivityLevelDebug, api.ActivityKindList, account, func(e *api.ActivityEntry) {
		e.N, e.Ms = count(n), ms(d)
	})
}

// MailboxSynced: reconciled a mailbox, with the messages it added, whose
// flags changed and that it removed. A pass that changed nothing is debug.
func MailboxSynced(account, mailbox int64, added, changed, removed int, d time.Duration) Entry {
	level := api.ActivityLevelInfo
	if added == 0 && changed == 0 && removed == 0 {
		level = api.ActivityLevelDebug
	}
	return entry(level, api.ActivityKindMailboxsynced, account, func(e *api.ActivityEntry) {
		e.MailboxID, e.Ms = &mailbox, ms(d)
		e.Added, e.Changed, e.Removed = count(added), count(changed), count(removed)
	})
}

// IdleWoke: IDLE reported news for a mailbox.
func IdleWoke(account, mailbox int64) Entry {
	return entry(api.ActivityLevelDebug, api.ActivityKindIdle, account, func(e *api.ActivityEntry) {
		e.MailboxID = &mailbox
	})
}

// Fetched: fetched n message bodies of bytes in all, after d.
func Fetched(account int64, n int, bytes int64, d time.Duration) Entry {
	return entry(api.ActivityLevelDebug, api.ActivityKindFetched, account, func(e *api.ActivityEntry) {
		e.N, e.Bytes, e.Ms = count(n), &bytes, ms(d)
	})
}

// Pushed: replayed n offline changes to the server.
func Pushed(account int64, n int) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindPushed, account, func(e *api.ActivityEntry) {
		e.N = count(n)
	})
}

// DraftSaved: saved n drafts' copies on the server.
func DraftSaved(account int64, n int) Entry {
	return entry(api.ActivityLevelDebug, api.ActivityKindDraftsaved, account, func(e *api.ActivityEntry) {
		e.N = count(n)
	})
}

// Sent: the SMTP server accepted a message, after d.
func Sent(account int64, d time.Duration) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindSent, account, func(e *api.ActivityEntry) {
		e.Service, e.Ms = server(api.ActivityServerSMTP), ms(d)
	})
}

// SendFailed: a send failed with err. A failure that will be retried is a
// warning, one the server refused for good an error.
func SendFailed(account int64, err error, retried bool) Entry {
	level := api.ActivityLevelError
	if retried {
		level = api.ActivityLevelWarn
	}
	return entry(level, api.ActivityKindSendfailed, account, func(e *api.ActivityEntry) {
		e.Service = server(api.ActivityServerSMTP)
		failure(e, err)
	})
}

// SendLater: queued n messages to send later.
func SendLater(account int64, n int) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindSendlater, account, func(e *api.ActivityEntry) {
		e.N = count(n)
	})
}

// RulesApplied: rules acted on n new messages.
func RulesApplied(account int64, n int) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindRulesapplied, account, func(e *api.ActivityEntry) {
		e.N = count(n)
	})
}

// RemindFired: Remind Me brought n messages back.
func RemindFired(account int64, n int) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindRemindfired, account, func(e *api.ActivityEntry) {
		e.N = count(n)
	})
}

// Announced: announced n new messages.
func Announced(account int64, n int) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindAnnounced, account, func(e *api.ActivityEntry) {
		e.N = count(n)
	})
}

// Erased: erased n old messages from a Trash or Junk mailbox.
func Erased(account, mailbox int64, n int) Entry {
	return entry(api.ActivityLevelInfo, api.ActivityKindErased, account, func(e *api.ActivityEntry) {
		e.MailboxID, e.N = &mailbox, count(n)
	})
}

// PIMSynced: a contacts, calendar or tasks pass over s changed that many
// items, after d. A pass that changed nothing is debug.
func PIMSynced(account int64, s api.ActivityServer, changed int, d time.Duration) Entry {
	level := api.ActivityLevelInfo
	if changed == 0 {
		level = api.ActivityLevelDebug
	}
	return entry(level, api.ActivityKindPimsynced, account, func(e *api.ActivityEntry) {
		e.Service, e.Changed, e.Ms = server(s), count(changed), ms(d)
	})
}

// PIMRefused: s refused a contact, event or task change with err.
func PIMRefused(account int64, s api.ActivityServer, err error) Entry {
	return entry(api.ActivityLevelWarn, api.ActivityKindPimrefused, account, func(e *api.ActivityEntry) {
		e.Service = server(s)
		failure(e, err)
	})
}

// TokenRefreshed: refreshed the account's OAuth access token.
func TokenRefreshed(account int64) Entry {
	return entry(api.ActivityLevelDebug, api.ActivityKindTokenrefreshed, account, func(e *api.ActivityEntry) {
		e.Service = server(api.ActivityServerOauth)
	})
}

// Refreshed is the end of an OAuth token refresh: TokenRefreshed, or
// SignInNeeded when the provider refused the grant, or Failed.
func Refreshed(account int64, err error) Entry {
	if err == nil {
		return TokenRefreshed(account)
	}
	if c, _, _ := Classify(err); c == api.ErrorClassAuth {
		return SignInNeeded(account, api.ActivityServerOauth, err)
	}
	return Lost(account, api.ActivityServerOauth, err)
}

// SignInNeeded: s refused the account's credentials with err.
// The class is auth unless err says better: a missing password or a
// refusal known only by a sentinel error is a refused sign-in too.
func SignInNeeded(account int64, s api.ActivityServer, err error) Entry {
	return entry(api.ActivityLevelError, api.ActivityKindSigninneeded, account, func(e *api.ActivityEntry) {
		e.Service = server(s)
		failure(e, err)
		if *e.ErrorClass == api.ErrorClassInternal {
			auth := api.ErrorClassAuth
			e.ErrorClass = &auth
		}
	})
}

// Offline: s could not be reached (err); retried with backoff.
func Offline(account int64, s api.ActivityServer, err error) Entry {
	return entry(api.ActivityLevelWarn, api.ActivityKindOffline, account, func(e *api.ActivityEntry) {
		e.Service = server(s)
		failure(e, err)
	})
}

// Failed: other work with s failed (err); retried with backoff.
func Failed(account int64, s api.ActivityServer, err error) Entry {
	return entry(api.ActivityLevelWarn, api.ActivityKindFailed, account, func(e *api.ActivityEntry) {
		e.Service = server(s)
		failure(e, err)
	})
}

// Lost is Offline when err says s could not be reached, and Failed
// otherwise: the sync loop's one entry for a session that ended.
func Lost(account int64, s api.ActivityServer, err error) Entry {
	switch c, _, _ := Classify(err); c {
	case api.ErrorClassNetwork, api.ErrorClassTimeout:
		return Offline(account, s, err)
	default:
		return Failed(account, s, err)
	}
}

// dropped counts n entries an account lost past PerSecond, or a
// subscription lost behind its buffer (account nil).
func dropped(account *int64, n int64) api.ActivityEntry {
	e := api.ActivityEntry{Level: api.ActivityLevelWarn, Kind: api.ActivityKindDropped, N: &n}
	if account != nil {
		id := *account
		e.AccountID = &id
	}
	return e
}

func entry(level api.ActivityLevel, kind api.ActivityKind, account int64, fill func(*api.ActivityEntry)) Entry {
	e := api.ActivityEntry{Level: level, Kind: kind, AccountID: &account}
	fill(&e)
	return Entry{e}
}

func failure(e *api.ActivityEntry, err error) {
	class, code, imapCode := Classify(err)
	e.ErrorClass, e.Code, e.IMAPCode = &class, code, imapCode
}

func server(s api.ActivityServer) *api.ActivityServer { return &s }

func count(n int) *int64 {
	v := int64(n)
	return &v
}

func ms(d time.Duration) *int64 {
	v := d.Milliseconds()
	return &v
}
