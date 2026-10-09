// Package oauth signs maild in to OAuth providers (docs/design/accounts.md,
// Sign-in; ADR-0011): the protocol pieces (protocol.go), provider
// endpoints, and later the loopback flow and the token manager.
package oauth

import (
	"fmt"
	"time"
)

// Endpoint is a provider's OAuth endpoints and the scopes mail needs.
type Endpoint struct {
	AuthURL  string
	TokenURL string
	Scopes   []string
}

// Google is Google's endpoint for Gmail over IMAP and SMTP.
var Google = Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
	Scopes:   []string{"https://mail.google.com/"},
}

// The Google scopes of the services besides mail (ADR-0017).
const (
	GoogleContactsScope = "https://www.googleapis.com/auth/carddav"
	GoogleCalendarScope = "https://www.googleapis.com/auth/calendar"
	GoogleTasksScope    = "https://www.googleapis.com/auth/tasks"
)

// Token is what a token endpoint grants.
type Token struct {
	Access  string
	Refresh string    // empty when the response carries none (refreshes)
	Expiry  time.Time // zero when the response gives no lifetime
	Scope   string    // the scopes granted, space-separated; "" when the response does not say
}

// TokenError is a token endpoint's error response (RFC 6749 §5.2).
type TokenError struct {
	Code        string // such as invalid_grant
	Description string
}

func (e *TokenError) Error() string {
	if e.Description == "" {
		return "oauth: " + e.Code
	}
	return fmt.Sprintf("oauth: %s: %s", e.Code, e.Description)
}

// ChallengeError is a mail server's XOAUTH2 failure details.
type ChallengeError struct {
	Status  string // HTTP-like status, such as "401"
	Schemes string
	Scope   string
}

func (e *ChallengeError) Error() string {
	return fmt.Sprintf("xoauth2: refused with status %s (scope %s)", e.Status, e.Scope)
}
