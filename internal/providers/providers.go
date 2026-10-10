// Package providers holds what Frostmail knows about mail providers: their
// servers, the sign-in kinds they accept, and the quirks the sync engine
// honors (docs/design/accounts.md, Provider profiles). Code asks the
// profile, never the host name.
package providers

import (
	"strings"

	"github.com/frostyard/frostmail/api"
)

// Server is one server's connection settings without the user name.
type Server struct {
	Host string
	Port int
	TLS  api.TLSMode
}

// Quirks are a provider's deviations the sync engine works around.
type Quirks struct {
	// Gmail selects the Gmail sync path (ADR-0012).
	Gmail bool
	// SavesSent: the server files sent mail itself, so maild does not append
	// a Sent copy.
	SavesSent bool
	// NoQResync: QRESYNC is not used even when advertised.
	NoQResync bool
	// SilentEnable: ENABLE gets no ENABLED reply; CONDSTORE counts as
	// enabled when advertised.
	SilentEnable bool
	// MaxConnections caps the account's simultaneous connections (0: the
	// engine's default).
	MaxConnections int
}

// Profile is one provider's settings.
type Profile struct {
	Kind api.AccountKind
	IMAP Server
	SMTP Server
	// Auth lists the sign-in kinds the provider accepts, preferred first.
	Auth []api.AuthKind
	// OAuth names the OAuth provider for oauth2 sign-in ("" for none).
	OAuth string
	// Domains are the address domains that belong to the provider.
	Domains []string
	Quirks  Quirks
	// DAV is where the provider keeps contacts, calendars and tasks.
	DAV DAV
}

// DAV is a provider's contacts, calendar and tasks endpoints (ADR-0017).
type DAV struct {
	// Contacts and Calendar are where CardDAV and CalDAV discovery start;
	// "" when the provider has none (or for generic accounts, which
	// discover from the address's domain).
	Contacts string
	Calendar string
	// Tasks is "google" for the Google Tasks API, "none" when the provider
	// has no tasks Frostmail can reach, and "" for task lists over CalDAV.
	Tasks string
	// NoTasks says why Tasks is "none", for Settings to show.
	NoTasks string
	// Schedules is a CalDAV server that tells an organizer of the user's
	// answer itself when a copy its scheduling delivered changes (RFC
	// 6638), so maild mails no iTIP reply of its own (ADR-0022).
	Schedules bool
	// SchedulesCopies is a server that also tells the organizer about a
	// copy a client creates from an emailed invitation: Google does
	// (verified 2026-10-09), iCloud does not.
	SchedulesCopies bool
	// Trusted are the domains, with their subdomains, that discovery may
	// send credentials to besides the one it started at: a principal or
	// home set on another host.
	Trusted []string
}

// GoogleTasksURL is the Google Tasks API's base URL.
const GoogleTasksURL = "https://tasks.googleapis.com/tasks/v1/"

var profiles = []Profile{
	{
		Kind:    api.AccountKindGmail,
		IMAP:    Server{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS},
		SMTP:    Server{Host: "smtp.gmail.com", Port: 465, TLS: api.TLSModeTLS},
		Auth:    []api.AuthKind{api.AuthKindOAuth2, api.AuthKindPassword},
		OAuth:   "google",
		Domains: []string{"gmail.com", "googlemail.com"},
		Quirks:  Quirks{Gmail: true, SavesSent: true, NoQResync: true, MaxConnections: 10},
		DAV: DAV{
			Contacts:        "https://www.googleapis.com/.well-known/carddav",
			Calendar:        "https://apidata.googleusercontent.com/caldav/v2/",
			Tasks:           "google",
			Schedules:       true,
			SchedulesCopies: true,
			Trusted:         []string{"googleapis.com", "googleusercontent.com"},
		},
	},
	{
		Kind:    api.AccountKindICloud,
		IMAP:    Server{Host: "imap.mail.me.com", Port: 993, TLS: api.TLSModeTLS},
		SMTP:    Server{Host: "smtp.mail.me.com", Port: 587, TLS: api.TLSModeStartTLS},
		Auth:    []api.AuthKind{api.AuthKindPassword},
		Domains: []string{"icloud.com", "me.com", "mac.com"},
		Quirks:  Quirks{NoQResync: true, SilentEnable: true},
		DAV: DAV{
			Contacts: "https://contacts.icloud.com/",
			Calendar: "https://caldav.icloud.com/",
			// Since iOS 13 and macOS 10.15 (2019), Reminders keeps its lists
			// where only Apple's apps reach them. CalDAV still lists the lists
			// left from before, frozen and named with "⚠️": a reminder made
			// today never appears there (probed on the user's account,
			// 2026-10-10).
			Tasks:     "none",
			NoTasks:   "Apple Reminders can't be reached by apps outside Apple's (since iOS 13).",
			Schedules: true,
			Trusted:   []string{"icloud.com"},
		},
	},
}

// ForKind returns the profile of an account kind; generic IMAP accounts
// (and kinds without a profile yet) get a profile with no servers and no
// quirks.
func ForKind(kind api.AccountKind) Profile {
	for _, p := range profiles {
		if p.Kind == kind {
			return p
		}
	}
	return Profile{Kind: kind, Auth: []api.AuthKind{api.AuthKindPassword}}
}

// ForAddress returns the profile whose domains include the address's
// domain; ok is false for other addresses.
func ForAddress(email string) (Profile, bool) {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return Profile{}, false
	}
	domain := strings.ToLower(email[at+1:])
	for _, p := range profiles {
		for _, d := range p.Domains {
			if d == domain {
				return p, true
			}
		}
	}
	return Profile{}, false
}

// TrustsHost reports whether host (without a port) is one of the profile's
// DAV domains or a subdomain of one.
func (p Profile) TrustsHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, d := range p.DAV.Trusted {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
