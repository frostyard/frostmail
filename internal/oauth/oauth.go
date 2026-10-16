// Package oauth signs maild in to OAuth providers (docs/design/accounts.md,
// Sign-in; ADR-0011): the protocol pieces (protocol.go), provider
// endpoints, and later the loopback flow and the token manager.
package oauth

import (
	"fmt"
	"net/url"
	"time"
)

// Endpoint is a provider's OAuth endpoints and the scopes mail needs.
type Endpoint struct {
	AuthURL  string
	TokenURL string
	Scopes   []string
	// Params are the authorization URL's parameters besides the flow's
	// own: how the provider is asked for a refresh token and a prompt.
	Params url.Values
	// ClientID is the provider's built-in public client, used when the user
	// stored none; "" means the user brings their own (ADR-0011).
	ClientID string
	// Loopback is the redirect's host: "127.0.0.1" ("" too), or
	// "localhost" for a client registered with it (Microsoft's).
	Loopback string
	// TokenScopes are sent with every token request for mail. Microsoft
	// grants a token for one audience, the one these scopes name
	// (ADR-0039); Google needs none.
	TokenScopes []string
	// Audiences are the provider's audiences besides mail's, by name, with
	// the prefix of their scopes: a token for one names the granted scopes
	// with that prefix (Microsoft's Graph).
	Audiences map[string]string
}

// AudienceGraph is Microsoft Graph's audience: contacts and calendars
// (ADR-0039, ADR-0040).
const AudienceGraph = "graph"

// Microsoft Graph's scopes for the services besides mail.
const (
	MicrosoftContactsScope = "https://graph.microsoft.com/Contacts.ReadWrite"
	MicrosoftCalendarScope = "https://graph.microsoft.com/Calendars.ReadWrite"
)

// Google is Google's endpoint for Gmail over IMAP and SMTP.
var Google = Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
	Scopes:   []string{"https://mail.google.com/"},
	Params:   url.Values{"access_type": {"offline"}, "prompt": {"consent"}},
}

// MicrosoftClientID is Frostmail's public client in Microsoft Entra
// (ADR-0039): any directory and personal accounts, the redirect
// http://localhost, no secret.
const MicrosoftClientID = "8007f9b9-a6b6-4029-8295-3e6af287317b"

// Outlook's audience: IMAP and SMTP over XOAUTH2 (probed 2026-10-15).
const (
	MicrosoftIMAPScope = "https://outlook.office.com/IMAP.AccessAsUser.All"
	MicrosoftSMTPScope = "https://outlook.office.com/SMTP.Send"
)

// Microsoft is Microsoft's endpoint for Outlook.com and Microsoft 365
// mail over IMAP and SMTP, at the common authority. A sign-in asks for
// offline access, and every token request names Outlook's audience.
var Microsoft = Endpoint{
	AuthURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL:    "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	Scopes:      []string{MicrosoftIMAPScope, MicrosoftSMTPScope, "offline_access"},
	Params:      url.Values{"prompt": {"select_account"}},
	ClientID:    MicrosoftClientID,
	Loopback:    "localhost",
	TokenScopes: []string{MicrosoftIMAPScope, MicrosoftSMTPScope, "offline_access"},
	Audiences:   map[string]string{AudienceGraph: "https://graph.microsoft.com/"},
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
