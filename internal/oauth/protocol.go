package oauth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// The OAuth protocol pieces maild needs: PKCE (RFC 7636), the authorization
// URL, token responses (RFC 6749) and SASL XOAUTH2 for IMAP and SMTP.

// verifierBytes is the entropy of a PKCE code verifier: 32 bytes, which
// unpadded base64url renders as the 43 characters RFC 7636 §4.1 asks for.
const verifierBytes = 32

// NewVerifier reads exactly 32 random bytes from r (crypto/rand.Reader in
// production) and returns them as a PKCE code verifier: unpadded base64url,
// 43 characters. A short read is an error.
func NewVerifier(r io.Reader) (string, error) {
	buf := make([]byte, verifierBytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("oauth: read verifier entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Challenge is the S256 code challenge of a verifier (RFC 7636 §4.2): the
// unpadded base64url of its SHA-256.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthURL is the authorization URL the user opens: e.AuthURL with the
// authorization-code and PKCE parameters, the endpoint's scopes joined by a
// space, offline access with consent, and login_hint only when it is not
// empty.
func AuthURL(e Endpoint, clientID, redirectURI, state, challenge, loginHint string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(e.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}
	if loginHint != "" {
		q.Set("login_hint", loginHint)
	}
	return e.AuthURL + "?" + q.Encode()
}

// tokenReply is a token endpoint's JSON body (RFC 6749 §5.1 and §5.2).
type tokenReply struct {
	Access      string `json:"access_token"`
	Refresh     string `json:"refresh_token"`
	ExpiresIn   int64  `json:"expires_in"`
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

// ParseToken reads a token endpoint's response body with its HTTP status and
// the current time. A body that carries an error is a *TokenError whatever
// the status; any other non-200 status, an undecodable body, a missing access
// token or a negative lifetime is a plain error. A lifetime of zero or a
// missing one leaves Expiry zero.
func ParseToken(body []byte, status int, now time.Time) (Token, error) {
	var reply tokenReply
	if err := json.Unmarshal(body, &reply); err != nil {
		return Token{}, fmt.Errorf("oauth: token endpoint reply is not JSON: %w", err)
	}
	if reply.Code != "" {
		return Token{}, &TokenError{Code: reply.Code, Description: reply.Description}
	}
	if status != 200 {
		return Token{}, fmt.Errorf("oauth: token endpoint answered HTTP %d", status)
	}
	if reply.Access == "" {
		return Token{}, errors.New("oauth: token response has no access token")
	}
	if reply.ExpiresIn < 0 {
		return Token{}, errors.New("oauth: token response has a negative expiry")
	}
	tok := Token{Access: reply.Access, Refresh: reply.Refresh}
	if reply.ExpiresIn > 0 {
		tok.Expiry = now.Add(time.Duration(reply.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// XOAuth2 is the SASL XOAUTH2 initial response for IMAP AUTHENTICATE and
// SMTP AUTH (the auth=Bearer line, with the trailing separators).
func XOAuth2(user, accessToken string) []byte {
	return []byte("user=" + user + "\x01auth=Bearer " + accessToken + "\x01\x01")
}

// xoauth2Reply is the JSON a mail server sends back when XOAUTH2 fails.
type xoauth2Reply struct {
	Status  string `json:"status"`
	Schemes string `json:"schemes"`
	Scope   string `json:"scope"`
}

// ParseXOAuth2Error reads a mail server's XOAUTH2 failure details, which
// arrive either as JSON or as standard base64 of that JSON. Anything that
// does not decode, or carries no status, is a plain error.
func ParseXOAuth2Error(b []byte) error {
	payload := bytes.TrimSpace(b)
	if len(payload) > 0 && payload[0] != '{' {
		decoded, err := base64.StdEncoding.DecodeString(string(payload))
		if err != nil {
			return errors.New("xoauth2: unreadable server error")
		}
		payload = decoded
	}
	var reply xoauth2Reply
	if err := json.Unmarshal(payload, &reply); err != nil || reply.Status == "" {
		return errors.New("xoauth2: unreadable server error")
	}
	return &ChallengeError{Status: reply.Status, Schemes: reply.Schemes, Scope: reply.Scope}
}
