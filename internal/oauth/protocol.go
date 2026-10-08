package oauth

import (
	"errors"
	"io"
	"time"
)

// The OAuth protocol pieces maild needs: PKCE (RFC 7636), the authorization
// URL, token responses (RFC 6749) and SASL XOAUTH2. Task T-0051 implements
// them; the stubs do nothing useful.

var errNotYet = errors.New("oauth: not implemented")

// NewVerifier returns a PKCE code verifier.
func NewVerifier(_ io.Reader) (string, error) { return "", errNotYet }

// Challenge is the S256 challenge of a verifier.
func Challenge(_ string) string { return "" }

// AuthURL is the authorization URL the user opens.
func AuthURL(_ Endpoint, _, _, _, _, _ string) string { return "" }

// ParseToken reads a token endpoint's response.
func ParseToken(_ []byte, _ int, _ time.Time) (Token, error) { return Token{}, errNotYet }

// XOAuth2 is the SASL XOAUTH2 initial response.
func XOAuth2(_, _ string) []byte { return nil }

// ParseXOAuth2Error reads a server's XOAUTH2 failure details.
func ParseXOAuth2Error(_ []byte) error { return errNotYet }
