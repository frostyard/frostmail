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
}

var profiles = []Profile{
	{
		Kind:    api.AccountKindGmail,
		IMAP:    Server{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS},
		SMTP:    Server{Host: "smtp.gmail.com", Port: 465, TLS: api.TLSModeTLS},
		Auth:    []api.AuthKind{api.AuthKindOAuth2, api.AuthKindPassword},
		OAuth:   "google",
		Domains: []string{"gmail.com", "googlemail.com"},
		Quirks:  Quirks{Gmail: true, SavesSent: true, NoQResync: true, MaxConnections: 10},
	},
	{
		Kind:    api.AccountKindICloud,
		IMAP:    Server{Host: "imap.mail.me.com", Port: 993, TLS: api.TLSModeTLS},
		SMTP:    Server{Host: "smtp.mail.me.com", Port: 587, TLS: api.TLSModeStartTLS},
		Auth:    []api.AuthKind{api.AuthKindPassword},
		Domains: []string{"icloud.com", "me.com", "mac.com"},
		Quirks:  Quirks{NoQResync: true, SilentEnable: true},
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
